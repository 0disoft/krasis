//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// ConsoleWorker owns exactly one job-bound console process. It is a launch
// primitive, not an input authorization endpoint. The service must close the
// authenticated input channel and finish recovery before calling Close.
type ConsoleWorker struct {
	mu           sync.Mutex
	closed       bool
	job, process windows.Handle
	image        *os.File
	done         chan struct{}
	once         sync.Once
	err          error
}

// LaunchConsoleWorker is service-only. Both image digest and agent authority
// must originate in the trusted installation/pipe boundary. No caller-selected
// path, PID, session, command, desktop or environment is accepted.
// It is not wired into the resident agent or installer yet.
func LaunchConsoleWorker(ctx context.Context, agent *Authority, digest [sha256.Size]byte) (*ConsoleWorker, error) {
	if ctx == nil || ctx.Err() != nil || agent == nil || agent.kind != authorityAgent || agent.check(false) != nil || digest == ([sha256.Size]byte{}) {
		return nil, ErrAuthority
	}
	type result struct {
		worker *ConsoleWorker
		err    error
	}
	ready := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		// An impersonation cleanup failure must never return this thread to Go.
		clean := true
		defer func() {
			if clean {
				runtime.UnlockOSThread()
			}
		}()
		undo, err := launchPrivileges()
		if err != nil {
			clean = false
			ready <- result{err: ErrAuthority}
			return
		}
		worker, err := launchConsole(ctx, agent, digest)
		if undo() != nil {
			clean = false
			if worker != nil {
				_ = worker.Close()
				worker = nil
			}
			err = ErrAuthority
		}
		ready <- result{worker, err}
	}()
	value := <-ready
	if value.err != nil {
		return nil, ErrAuthority
	}
	if ctx.Err() != nil {
		_ = value.worker.Close()
		return nil, ErrAuthority
	}
	go value.worker.watch(ctx, agent)
	return value.worker, nil
}

func launchConsole(ctx context.Context, agent *Authority, digest [sha256.Size]byte) (*ConsoleWorker, error) {
	self, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		return nil, ErrAuthority
	}
	defer self.close()
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return nil, ErrAuthority
	}
	path := filepath.Join(root, "Krasis", "Input", "krasis-input.exe")
	if !self.system || self.session != 0 || self.integrity != 0x4000 || self.restricted || self.appContainer || !strings.EqualFold(self.image, path) {
		return nil, ErrAuthority
	}
	image, err := lockImage(path, digest)
	if err != nil {
		return nil, ErrAuthority
	}
	worker := &ConsoleWorker{image: image, done: make(chan struct{})}
	accepted := false
	defer func() {
		if !accepted {
			_ = worker.Close()
		}
	}()
	session := windows.WTSGetActiveConsoleSessionId()
	if session == 0 || session == 0xffffffff || agent.check(false) != nil {
		return nil, ErrAuthority
	}
	token, err := consoleToken(session)
	if err != nil {
		return nil, ErrAuthority
	}
	defer token.Close()
	worker.job, err = makeWorkerJob()
	if err != nil {
		return nil, ErrAuthority
	}
	worker.process, err = createConsoleProcess(ctx, token, path, worker.job, func() bool {
		return windows.WTSGetActiveConsoleSessionId() == session && agent.check(false) == nil
	})
	if err != nil {
		return nil, ErrAuthority
	}
	accepted = true
	return worker, nil
}

func (worker *ConsoleWorker) watch(ctx context.Context, agent *Authority) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-worker.done:
			return
		case <-ctx.Done():
			_ = worker.Close()
			return
		case <-ticker.C:
			if worker.exited() {
				_ = worker.Close()
				return
			}
			if agent.check(false) != nil {
				// The channel's independent recovery has a bounded opportunity
				// to release inputs before we forcibly reap the owned process.
				timer := time.NewTimer(RecoveryTimeout + 100*time.Millisecond)
				select {
				case <-worker.done:
				case <-ctx.Done():
				case <-timer.C:
				}
				timer.Stop()
				_ = worker.Close()
				return
			}
		}
	}
}

func (worker *ConsoleWorker) exited() bool {
	worker.mu.Lock()
	defer worker.mu.Unlock()
	if worker.closed {
		return true
	}
	if worker.process == 0 {
		return false
	}
	state, err := windows.WaitForSingleObject(worker.process, 0)
	return err != nil || state != uint32(windows.WAIT_TIMEOUT)
}

// Close is idempotent and waits for the exact owned child, never a PID lookup.
// Job close also covers service crash and a child that has not resumed yet.
func (worker *ConsoleWorker) Close() error {
	if worker == nil {
		return nil
	}
	worker.once.Do(func() {
		worker.mu.Lock()
		defer worker.mu.Unlock()
		worker.closed = true
		if worker.done != nil {
			close(worker.done)
		}
		if worker.job != 0 && windows.CloseHandle(worker.job) != nil {
			worker.err = ErrAuthority
		}
		if worker.process != 0 {
			state, err := windows.WaitForSingleObject(worker.process, 5000)
			if err != nil || state != windows.WAIT_OBJECT_0 {
				worker.err = ErrAuthority
			}
			if windows.CloseHandle(worker.process) != nil {
				worker.err = ErrAuthority
			}
		}
		if worker.image != nil && worker.image.Close() != nil {
			worker.err = ErrAuthority
		}
	})
	return worker.err
}
