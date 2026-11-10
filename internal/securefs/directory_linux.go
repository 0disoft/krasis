//go:build linux

package securefs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

// Directory owns a pinned directory and, for RemoveSelf, its original parent.
// path is a display name only; no operation reopens a child through that path.
type Directory struct {
	path   string
	mu     sync.RWMutex
	file   *os.File
	parent *os.File
	name   string
}

func OpenDirectory(path string) (*Directory, error)     { return openDirectory(path) }
func OpenReadDirectory(path string) (*Directory, error) { return openDirectory(path) }

func openDirectory(path string) (*Directory, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Resolve the parent in one kernel operation, refusing symlinks in every
	// component. Do not fall back on older kernels or under restrictive seccomp.
	parent, err := linuxOpen(unix.AT_FDCWD, filepath.Dir(absolute), unix.O_RDONLY|unix.O_DIRECTORY, 0, false)
	if err != nil {
		return nil, fmt.Errorf("open secure directory parent: %w", err)
	}
	name := filepath.Base(absolute)
	if absolute == "/" {
		name = "."
	}
	fd, err := linuxOpen(parent, name, unix.O_RDONLY|unix.O_DIRECTORY, 0, true)
	if err != nil {
		unix.Close(parent)
		return nil, fmt.Errorf("open secure directory: %w", err)
	}
	return &Directory{path: absolute, file: os.NewFile(uintptr(fd), absolute), parent: os.NewFile(uintptr(parent), filepath.Dir(absolute)), name: name}, nil
}

func linuxOpen(root int, name string, flags int, mode uint32, beneath bool) (int, error) {
	resolve := uint64(unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS)
	if beneath {
		resolve |= unix.RESOLVE_BENEATH
	}
	fd, err := unix.Openat2(root, name, &unix.OpenHow{Flags: uint64(flags | unix.O_CLOEXEC | unix.O_NOFOLLOW), Mode: uint64(mode), Resolve: resolve})
	// O_NOFOLLOW|O_DIRECTORY can report ENOTDIR instead of ELOOP for a
	// symlink component. Both a symlink and an actual non-directory component
	// are unsafe traversal; ENOENT must remain a recognizable missing path.
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOTDIR) {
		return -1, ErrUnsafePath
	}
	if errors.Is(err, unix.ENOSYS) {
		return -1, fmt.Errorf("%w: %w", ErrUnsupportedPlatform, err)
	}
	return fd, err
}

func (directory *Directory) Close() error {
	if directory == nil {
		return nil
	}
	directory.mu.Lock()
	defer directory.mu.Unlock()
	if directory.file == nil {
		return nil
	}
	err := directory.file.Close()
	directory.file = nil
	if directory.parent != nil {
		err = errors.Join(err, directory.parent.Close())
		directory.parent = nil
	}
	return err
}

// Handle borrows the fd. The caller must keep Directory open until its native
// operation finishes. Internal operations use pin instead, serializing Close.
func (directory *Directory) Handle() uintptr {
	if directory == nil {
		return 0
	}
	directory.mu.RLock()
	defer directory.mu.RUnlock()
	if directory.file == nil {
		return 0
	}
	return directory.file.Fd()
}

// A successful pin caller must RUnlock once the root-relative syscall ends.
func (directory *Directory) pin() (int, error) {
	if directory == nil {
		return -1, os.ErrClosed
	}
	directory.mu.RLock()
	if directory.file == nil {
		directory.mu.RUnlock()
		return -1, os.ErrClosed
	}
	return int(directory.file.Fd()), nil
}

func (directory *Directory) Open(name string) (*os.File, error) {
	return directory.OpenObserved(name, nil)
}
func (directory *Directory) OpenObserved(name string, observe func(string)) (*os.File, error) {
	return directory.openFile(name, unix.O_RDONLY, 0, observe)
}
func (directory *Directory) OpenReadWrite(name string) (*os.File, error) {
	return directory.openFile(name, unix.O_RDWR, 0, nil)
}

// Linux locks are advisory; unlike Windows share modes these handles cannot
// prevent unlink or rename by another process. Callers must use their native
// locking protocol and a directory protected from untrusted writers.
func (directory *Directory) OpenLock(name string, readOnly bool) (*os.File, error) {
	if readOnly {
		return directory.Open(name)
	}
	return directory.OpenReadWrite(name)
}
func (directory *Directory) CreateLock(name string, mode os.FileMode) (*os.File, error) {
	return directory.CreateExclusive(name, mode)
}
func (directory *Directory) CreateExclusive(name string, mode os.FileMode) (*os.File, error) {
	return directory.openFile(name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL, uint32(mode.Perm()), nil)
}

func (directory *Directory) openFile(name string, flags int, mode uint32, observe func(string)) (*os.File, error) {
	if err := validateLeaf(name); err != nil {
		return nil, err
	}
	root, err := directory.pin()
	if err != nil {
		return nil, err
	}
	fd, err := func() (int, error) {
		defer directory.mu.RUnlock()
		if observe != nil {
			observe("native")
		}
		// O_NONBLOCK prevents an attacker-controlled FIFO from hanging before
		// fstat can reject it; it has no effect on regular-file IO.
		return linuxOpen(root, name, flags|unix.O_NONBLOCK, mode, true)
	}()
	if err != nil {
		return nil, err
	}
	if observe != nil {
		observe("info")
	}
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil {
		unix.Close(fd)
		return nil, err
	}
	if err := regularFile(&info); err != nil {
		unix.Close(fd)
		return nil, err
	}
	if observe != nil {
		observe("wrap")
	}
	return os.NewFile(uintptr(fd), filepath.Join(directory.path, name)), nil
}

func regularFile(info *unix.Stat_t) error {
	if info.Mode&unix.S_IFMT != unix.S_IFREG {
		return ErrUnsafePath
	}
	if info.Nlink != 1 {
		return ErrHardLink
	}
	return nil
}

func (directory *Directory) CreateTemporary(prefix string, mode os.FileMode) (*os.File, string, error) {
	for attempt := 0; attempt < 16; attempt++ {
		name, err := temporaryName(prefix)
		if err != nil {
			return nil, "", err
		}
		file, err := directory.CreateExclusive(name, mode)
		if err == nil {
			return file, name, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", errors.New("secure temporary name collision limit reached")
}

func (directory *Directory) CreateDirectory(name string, mode os.FileMode) (*Directory, error) {
	if err := validateLeaf(name); err != nil {
		return nil, err
	}
	root, err := directory.pin()
	if err != nil {
		return nil, err
	}
	defer directory.mu.RUnlock()
	if err := unix.Mkdirat(root, name, uint32(mode.Perm())); err != nil {
		return nil, err
	}
	fd, err := linuxOpen(root, name, unix.O_RDONLY|unix.O_DIRECTORY, 0, true)
	if err != nil {
		return nil, err
	}
	parent, err := unix.FcntlInt(uintptr(root), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	path := filepath.Join(directory.path, name)
	return &Directory{path: path, file: os.NewFile(uintptr(fd), path), parent: os.NewFile(uintptr(parent), directory.path), name: name}, nil
}
