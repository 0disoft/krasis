//go:build windows

package inputworker

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/wininput"
	"golang.org/x/sys/windows"
)

type preparedNativeChild struct {
	owner   *crashguard.LocalInputPreparation
	process *os.Process // Exact launched owner, never reopened by PID.
	claimed atomic.Bool
}

type preparedNativePair struct {
	children [2]*preparedNativeChild
}

// The service-authenticated journal worker launches within its existing job.
// Children load/validate their fixed installation and wait on stdin, with no
// inherited authority handles, lease, desktop scope or input transport.
func prepareNativePair(ctx context.Context) (*preparedNativePair, error) {
	p := &preparedNativePair{}
	var joined sync.WaitGroup
	var failures [2]error
	for i, mode := range [2]string{"--native-input-worker", "--native-release-worker"} {
		joined.Go(func() {
			root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
			if err != nil {
				failures[i] = ErrAuthority
				return
			}
			command := exec.Command(filepath.Join(root, "Krasis", "Input", "krasis-input.exe"), mode)
			command.Dir = filepath.Dir(command.Path)
			command.Env = []string{}
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			owner, err := crashguard.PrepareLocalInputCommandContext(ctx, command)
			failures[i] = err
			if owner != nil {
				p.children[i] = &preparedNativeChild{owner: owner, process: command.Process}
			}
		})
	}
	joined.Wait()
	if err := errors.Join(failures[0], failures[1]); err != nil {
		return nil, errors.Join(err, p.Close())
	}
	return p, nil
}

func (p *preparedNativePair) Close() error {
	if p == nil {
		return nil
	}
	var joined sync.WaitGroup
	var failures [2]error
	for i, child := range p.children {
		if child != nil {
			joined.Go(func() { failures[i] = child.owner.Close() })
		}
	}
	joined.Wait()
	if err := errors.Join(failures[0], failures[1]); err != nil {
		return errors.Join(ErrRecovery, err)
	}
	return nil
}

func (p *preparedNativePair) bind(ctx context.Context, release bool, handles []windows.Handle, bootstrap []byte) (*isolatedInput, error) {
	i := 0
	if release {
		i = 1
	}
	if p == nil || p.children[i] == nil || len(handles) != 4 || len(bootstrap) != nativeBootstrapSize || ctx == nil || ctx.Err() != nil {
		return nil, ErrAuthority
	}
	child := p.children[i]
	if !child.claimed.CompareAndSwap(false, true) {
		return nil, ErrAuthority
	}
	// Only now can the already approved lease's agent/service/image pins be
	// duplicated into this exact child. Partial transfer stays owned by that
	// child and is retired before any replacement can be admitted.
	data := append([]byte(nil), bootstrap...)
	var transferErr error
	err := child.process.WithHandle(func(target uintptr) {
		var values [4]uint64
		for i, source := range handles {
			var transferred windows.Handle
			if ctx.Err() != nil {
				transferErr = ctx.Err()
				return
			}
			if err := windows.DuplicateHandle(windows.CurrentProcess(), source, windows.Handle(target), &transferred, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
				transferErr = err
				return
			}
			values[i] = uint64(transferred)
		}
		binary.BigEndian.PutUint64(data[8:16], values[0])
		binary.BigEndian.PutUint64(data[16:24], values[1])
		binary.BigEndian.PutUint64(data[bootstrapSize:bootstrapSize+8], values[2])
		binary.BigEndian.PutUint64(data[bootstrapSize+8:], values[3])
	})
	if err != nil || transferErr != nil {
		return nil, errors.Join(ErrAuthority, err, transferErr, child.owner.Close())
	}
	process, err := child.owner.Bind(ctx, data)
	if err != nil {
		return nil, err
	}
	client, err := newPortableClient(ctx, process)
	if err != nil {
		return nil, errors.Join(err, process.Close())
	}
	return newIsolatedInput(client, process, wininput.NewInjectionPort()), nil
}
