//go:build linux

package launcher

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/unix"
)

// EnsureAgent starts only the sibling Linux agent, without a shell or PATH
// lookup. Its private API credentials and runtime response establish readiness;
// a successful fork alone does not mean that the agent initialized successfully.
func EnsureAgent(ctx context.Context, localData, executable string) error {
	if ctx == nil || !filepath.IsAbs(localData) || !filepath.IsAbs(executable) {
		return os.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	paths, err := productidentity.ResolveWindowsPaths(localData)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var startup *agentinstance.Lease
	for startup == nil {
		startup, err = agentinstance.AcquireControlStartup()
		if errors.Is(err, agentinstance.ErrAlreadyRunning) {
			if err := startupPause(ctx); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
	}
	defer startup.Close()
	if ready, err := agentReady(ctx, paths.Control); ready || err != nil {
		return err
	}
	lease, err := agentinstance.Acquire()
	if err != nil && !errors.Is(err, agentinstance.ErrAlreadyRunning) {
		return err
	}
	if lease != nil {
		if err := lease.Close(); err != nil {
			return err
		}
		if err := startSiblingAgent(executable, localData); err != nil {
			return err
		}
	}
	for {
		if err := startupPause(ctx); err != nil {
			return errors.Join(localapi.ErrUnavailable, err)
		}
		if ready, err := agentReady(ctx, paths.Control); ready || err != nil {
			return err
		}
	}
}

func startupPause(ctx context.Context) error {
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func agentReady(ctx context.Context, control string) (bool, error) {
	client, err := localapi.OpenClient(control)
	if err == nil {
		probe, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		var info localapi.RuntimeInfo
		info, err = client.Runtime(probe)
		if err == nil {
			if info.Protocol != localapi.AgentProtocol {
				return false, localapi.ErrProtocol
			}
			return true, nil
		}
	}
	if errors.Is(err, localapi.ErrUnavailable) || errors.Is(err, localapi.ErrTransport) || errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func startSiblingAgent(executable, localData string) error {
	base := filepath.Dir(executable)
	dir, err := securefs.OpenReadDirectory(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	var directory unix.Stat_t
	if err := unix.Fstat(int(dir.Handle()), &directory); err != nil {
		return err
	}
	if directory.Uid != 0 && directory.Uid != uint32(os.Geteuid()) || directory.Mode&0o022 != 0 {
		return errors.New("agent bundle directory is writable by another user")
	}
	image, err := dir.Open("krasis-agent")
	if err != nil {
		return err
	}
	defer image.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(int(image.Fd()), &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Mode&0o111 == 0 || stat.Mode&0o022 != 0 ||
		stat.Uid != 0 && stat.Uid != uint32(os.Geteuid()) {
		return errors.New("agent image is not a trusted executable")
	}
	// Pin the exact validated executable across exec. ExtraFiles maps this
	// descriptor to child fd 3, so rename/symlink races cannot substitute code.
	command := exec.Command("/proc/self/fd/3")
	command.Args[0] = filepath.Join(base, "krasis-agent")
	command.Dir = base
	command.ExtraFiles = []*os.File{image}
	// X11/Wayland, D-Bus and XDG variables must survive autostart unchanged.
	command.Env = append(os.Environ(), "XDG_DATA_HOME="+localData)
	return launchrecord.StartIndependent(command)
}
