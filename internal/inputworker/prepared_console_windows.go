//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"golang.org/x/sys/windows"
)

// Preparation owns only the job and the SYSTEM-only control pipe. No lease,
// agent input pipe or image is transferred until the existing bootstrap.
type preparedConsole struct {
	worker     *ConsoleWorker
	control    *pipeConn
	pid        uint32
	session    uint32
	profile    handoff.StartupProfile
	preparedAt time.Time
}

type consoleLauncher func(func(windows.Handle, uint32) error) (*ConsoleWorker, error)

func prepareAgentConsole(ctx context.Context, agent *Authority, digest [sha256.Size]byte, installation *Installation) (*preparedConsole, error) {
	return prepareConsole(ctx, func(prepare func(windows.Handle, uint32) error) (*ConsoleWorker, error) {
		return launchManagedConsole(ctx, agent, digest, prepare, installation)
	})
}

func prepareConsole(ctx context.Context, launch consoleLauncher) (prepared *preparedConsole, result error) {
	if ctx == nil || ctx.Err() != nil || launch == nil {
		return nil, ErrAuthority
	}
	p := &preparedConsole{}
	accepted := false
	defer func() {
		if !accepted && p.close() != nil {
			result = errors.Join(result, ErrRecovery)
		}
	}()
	started := time.Now()
	worker, err := launch(func(_ windows.Handle, pid uint32) error {
		p.pid, p.session = pid, windows.WTSGetActiveConsoleSessionId()
		var err error
		p.control, err = makePipeACL(workerPipeName(pid), "D:P(A;;GA;;;SY)")
		return err
	})
	p.profile.Record(handoff.ServiceLaunch, time.Since(started))
	if err != nil {
		return nil, ErrAuthority
	}
	p.worker = worker
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	started = time.Now()
	err = p.control.connect(startup)
	p.profile.Record(handoff.ServiceChildConnect, time.Since(started))
	if err != nil || startup.Err() != nil {
		return nil, ErrAuthority
	}
	p.preparedAt = time.Now()
	accepted = true
	return p, nil
}

func (prepared *preparedConsole) close() error {
	if prepared == nil {
		return nil
	}
	var err error
	if prepared.control != nil {
		err = prepared.control.Close()
	}
	if prepared.worker != nil {
		err = errors.Join(err, prepared.worker.Close())
	}
	return err
}
