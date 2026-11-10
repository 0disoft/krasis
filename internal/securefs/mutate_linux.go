//go:build linux

package securefs

import "golang.org/x/sys/unix"

func (directory *Directory) Replace(source, destination string) error {
	return directory.rename(source, destination, 0)
}

func (directory *Directory) PromoteNoReplace(source, destination string) error {
	return directory.rename(source, destination, unix.RENAME_NOREPLACE)
}

func (directory *Directory) rename(source, destination string, flags uint) error {
	if err := validateLeaf(source); err != nil {
		return err
	}
	if err := validateLeaf(destination); err != nil {
		return err
	}
	root, err := directory.pin()
	if err != nil {
		return err
	}
	defer directory.mu.RUnlock()
	fd, info, err := pinRegular(root, source)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if flags == 0 {
		var target unix.Stat_t
		if err := unix.Fstatat(root, destination, &target, unix.AT_SYMLINK_NOFOLLOW); err == nil {
			if err := regularFile(&target); err != nil {
				return err
			}
		} else if err != unix.ENOENT {
			return err
		}
	}
	if err := sameEntry(root, source, &info); err != nil {
		return err
	}
	// Neither endpoint can escape the pinned root. Linux has no rename-by-fd
	// or inode-conditional rename: namespace mutation requires a directory
	// protected from untrusted writers, even though opens remain race-safe.
	// Never emulate NOREPLACE with a check followed by a replacing rename.
	return unix.Renameat2(root, source, root, destination, flags)
}

func (directory *Directory) Remove(name string) error {
	if err := validateLeaf(name); err != nil {
		return err
	}
	root, err := directory.pin()
	if err != nil {
		return err
	}
	defer directory.mu.RUnlock()
	fd, info, err := pinRegular(root, name)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if err := sameEntry(root, name, &info); err != nil {
		return err
	}
	// unlinkat never follows the leaf. The inode check refuses an already
	// replaced entry; Linux offers no atomic compare-inode-and-unlink syscall.
	return unix.Unlinkat(root, name, 0)
}

func (directory *Directory) RemoveSelf() error {
	root, err := directory.pin()
	if err != nil {
		return err
	}
	defer directory.mu.RUnlock()
	if directory.parent == nil || directory.name == "." {
		return ErrUnsafePath
	}
	var info unix.Stat_t
	if err := unix.Fstat(root, &info); err != nil {
		return err
	}
	parent := int(directory.parent.Fd())
	if err := sameEntry(parent, directory.name, &info); err != nil {
		return err
	}
	return unix.Unlinkat(parent, directory.name, unix.AT_REMOVEDIR)
}

func pinRegular(root int, name string) (int, unix.Stat_t, error) {
	var info unix.Stat_t
	// O_PATH avoids opening devices or blocking on FIFOs. A symlink returned
	// by O_PATH|O_NOFOLLOW is rejected by the fstat type check below.
	fd, err := linuxOpen(root, name, unix.O_PATH, 0, true)
	if err != nil {
		return -1, info, err
	}
	err = unix.Fstat(fd, &info)
	if err == nil {
		err = regularFile(&info)
	}
	if err != nil {
		unix.Close(fd)
		return -1, info, err
	}
	return fd, info, nil
}

func sameEntry(root int, name string, expected *unix.Stat_t) error {
	var current unix.Stat_t
	if err := unix.Fstatat(root, name, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if current.Dev != expected.Dev || current.Ino != expected.Ino || current.Mode&unix.S_IFMT != expected.Mode&unix.S_IFMT {
		return ErrUnsafePath
	}
	if current.Mode&unix.S_IFMT == unix.S_IFREG {
		return regularFile(&current)
	}
	return nil
}
