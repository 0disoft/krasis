//go:build linux

package agentinstance

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func validName(name string) bool {
	return strings.HasPrefix(name, `Local\Krasis-Agent-Test-`) || name == productionName || name == controlName || name == `Local\Krasis-Control-Startup-v1` || name == `Local\Krasis-Log-agent-v1` || name == `Local\Krasis-Log-control-v1`
}

func acquire(name string) (*Lease, error) {
	if !validName(name) {
		return nil, errors.New("agent instance name is invalid")
	}
	dir, err := instanceDirectory()
	if err != nil {
		return nil, err
	}
	defer unix.Close(dir)
	return acquireAt(dir, name)
}

// A fixed per-UID namespace, independent of HOME, TMPDIR and XDG overrides,
// keeps independently launched CLI/agent processes on the same ownership gate.
// Never unlink lock files: a new inode would permit two simultaneous owners.
func instanceDirectory() (int, error) {
	parent, err := unix.Open("/tmp", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	defer unix.Close(parent)
	name := fmt.Sprintf("krasis-%d", os.Geteuid())
	if err := unix.Mkdirat(parent, name, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
		return -1, err
	}
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		unix.Close(fd)
		return -1, err
	}
	if stat.Uid != uint32(os.Geteuid()) || stat.Mode&0o7777 != 0o700 {
		unix.Close(fd)
		return -1, errors.New("agent instance directory is not private to this user")
	}
	return fd, nil
}

func acquireAt(dir int, name string) (*Lease, error) {
	filename := fmt.Sprintf("%x.lock", sha256.Sum256([]byte(name)))
	fd, err := unix.Openat(dir, filename, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	err = unix.Fstat(fd, &stat)
	if err == nil && (stat.Uid != uint32(os.Geteuid()) || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o7777 != 0o600 || stat.Nlink != 1) {
		err = errors.New("agent instance lock is not a private regular file")
	}
	if err == nil {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			err = ErrAlreadyRunning
		}
	}
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &Lease{close: func() error { return unix.Close(fd) }}, nil
}

func ControlRunning() (bool, error) {
	lease, err := acquire(controlName)
	if errors.Is(err, ErrAlreadyRunning) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, lease.Close()
}
