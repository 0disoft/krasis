//go:build windows

package inputworker

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// RunService starts only through SCM with a protected installation. It neither
// installs itself nor elevates a portable agent.
func RunService(installation *Installation) error {
	agent, service, err := installation.digests()
	if err != nil || !fixedInputProcess(service, false) {
		return ErrAuthority
	}
	managed, err := svc.IsWindowsService()
	if err != nil || !managed {
		return ErrAuthority
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return ErrAuthority
	}
	if svc.Run(ServiceName, &serviceHandler{check: func(args []string) error {
		return checkServiceStart(filepath.Join(root, "Krasis"), installImages{Agent: agent, Service: service}, args)
	}, run: func(ctx context.Context) error { return runInstalledService(ctx, installation, service) }}) != nil {
		return ErrAuthority
	}
	return nil
}

func fixedInputProcess(digest [32]byte, console bool) bool {
	return fixedInstalledInputProcess(digest, console, nil)
}

func fixedInstalledInputProcess(digest [32]byte, console bool, installation *Installation) bool {
	identity, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		return false
	}
	defer identity.close()
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil || !identity.system || identity.integrity != 0x4000 || identity.restricted || identity.appContainer || !strings.EqualFold(identity.image, filepath.Join(root, "Krasis", "Input", "krasis-input.exe")) {
		return false
	}
	if console {
		if !validConsoleToken(identity.token, windows.WTSGetActiveConsoleSessionId()) {
			return false
		}
	} else if identity.session != 0 {
		return false
	}
	image, err := pinIdentityImage(installation, identity.image, digest)
	if err != nil {
		return false
	}
	_ = image.Close()
	return true
}

// RunInstalledWorker cannot consume a caller-provided trust record.
func RunInstalledWorker(ctx context.Context, installation *Installation) error {
	return runInstalledWorker(ctx, installation, false)
}

// Only the service's bounded input preparation selects this fixed mode. The
// extra processes receive no agent handles, lease or input until bootstrap.
func RunInstalledPreparedWorker(ctx context.Context, installation *Installation) error {
	return runInstalledWorker(ctx, installation, true)
}

func runInstalledWorker(ctx context.Context, installation *Installation, prepareNative bool) error {
	var profile handoff.StartupProfile
	started := time.Now()
	agent, service, err := installation.workerDigests()
	if err != nil || !fixedInstalledInputProcess(service, true, installation) {
		return ErrAuthority
	}
	// Include the load/hash work performed before entering this function. The
	// span overlaps ServiceChildConnect; it must not be added to that span.
	profile.Record(handoff.WorkerInstalledValidation, installation.loadElapsed+time.Since(started))
	profile.RecordLoad(installation.loadElapsed)
	return runInstalledConsoleInputMode(ctx, agent, service, profile, installation, prepareNative)
}

func runServiceLoop(ctx context.Context, installation *Installation, ready func() error) (result error) {
	return runServiceLoopWithGate(ctx, installation, ready, nil)
}

func runServiceLoopWithGate(ctx context.Context, installation *Installation, ready func() error, admit func(context.Context) error, first ...*pipeConn) (result error) {
	agent, service, err := installation.digests()
	if err != nil {
		return ErrAuthority
	}
	var preparation serviceInputPreparation
	var lastPreparation time.Time
	var returned presentationReturn
	var initial *pipeConn
	if len(first) != 0 {
		initial = first[0]
	}
	listener, err := newServicePipeListener(pipeName, initial)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := preparation.close(); closeErr != nil {
			result = errors.Join(result, ErrRecovery, closeErr)
		}
		result = errors.Join(result, listener.Close())
	}()
	for ctx.Err() == nil {
		initial, err := listener.borrow()
		if err != nil {
			return err
		}
		readyFailed := false
		channel, err := preparation.accept(func() (*Channel, error) {
			return acceptAgentReadyOnPipe(ctx, agent, func() error {
				if ready != nil && ready() != nil {
					readyFailed = true
					return ErrAuthority
				}
				return nil
			}, installation, initial)
		})
		if readyFailed {
			return ErrAuthority
		}
		if errors.Is(err, ErrRecovery) {
			return err
		}
		if err == nil {
			started := time.Now()
			lease, prepare, leaseErr := receiveAgentRequest(ctx, channel)
			channel.serviceReadiness.Record(handoff.ServiceLeaseReceive, time.Since(started))
			if leaseErr == nil && !prepare {
				if err := preparation.admit(channel); err != nil {
					return err
				}
			}
			if admit != nil {
				started := time.Now()
				if err = admit(ctx); err != nil {
					_ = channel.Close()
					if errors.Is(err, ErrRecovery) {
						return err
					}
					continue
				}
				channel.serviceReadiness.Record(handoff.ServiceLoginAdmission, time.Since(started))
			}
			if leaseErr == nil && prepare {
				// Release the agent's optional-preparation mutex before native
				// construction. No lease or input authority is acknowledged here.
				err = acknowledgePreparation(ctx, channel)
				closeErr := channel.Close()
				if failure := serviceRecoveryFailure(err, closeErr); failure != nil {
					return failure
				}
				if err == nil && closeErr == nil {
					now := time.Now()
					if preparation.hint(ctx, lastPreparation, now, func(run context.Context) (*preparedConsole, error) {
						return prepareServiceConsole(run, installation, service)
					}) {
						lastPreparation = now
					}
				}
				// Preparation failures leave ordinary lease fallback usable.
				continue
			}
			if leaseErr == nil {
				if lease.Capture {
					_ = channel.Close()
					continue
				}
				var agentPID uint32
				pipe, ok := channel.stream.(*pipeConn)
				if !ok || windows.GetNamedPipeClientProcessId(pipe.handle, &agentPID) != nil ||
					(lease.Presentation && !returned.take(lease, agentPID, channel.authority.agentStarted, time.Now())) {
					_ = channel.Close()
					continue
				}
				if !lease.Presentation {
					returned = presentationReturn{}
				}
				candidate := takeInputPreparation(&preparation.next, lease)
				_, err = runPreparedAgentLease(ctx, channel, lease, agent, service, installation, candidate, func() {
					// Readiness has reached the agent before any next-input work.
					// Overlap one unbound set with active input as well as passive
					// presentation. No current lease, journal or agent handles enter
					// this set; its fixed expiry is not extended or renewed.
					if preparation.start(ctx, func(run context.Context) (*preparedConsole, error) {
						return prepareServiceConsole(run, installation, service)
					}) {
						lastPreparation = time.Now()
					}
				})
				if err == nil && !lease.Presentation {
					returned = presentationReturn{lease: lease, pid: agentPID, started: channel.authority.agentStarted, at: time.Now()}
				}
				if !lease.Presentation {
					// Reuse a still-live spare, keep an unfinished builder, or
					// replace an expired spare only after exact active retirement.
					if preparationErr := preparation.reenter(ctx, lease, err, func(run context.Context) (*preparedConsole, error) {
						return prepareServiceConsole(run, installation, service)
					}); preparationErr != nil {
						return serviceRecoveryFailure(preparationErr, channel.Close())
					}
				}
				if errors.Is(err, ErrRecovery) {
					return serviceRecoveryFailure(err, channel.Close())
				}
			} else {
				err = leaseErr
			}
			if closeErr := channel.Close(); closeErr != nil && err == nil {
				returned = presentationReturn{}
				err = closeErr
			}
		}
		// Recovery and the owned worker have already finished. A clean lease
		// needs no retry backoff; the next accept still blocks awaiting a client.
		if err == nil {
			continue
		}
		if closeErr := preparation.close(); closeErr != nil {
			return errors.Join(ErrRecovery, closeErr)
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}

type serviceHandler struct {
	check func([]string) error
	run   func(context.Context) error
}

func (handler *serviceHandler) Execute(args []string, changes <-chan svc.ChangeRequest, updates chan<- svc.Status) (bool, uint32) {
	updates <- svc.Status{State: svc.StartPending, CheckPoint: 1, WaitHint: 10000}
	if handler.run == nil || handler.check == nil && len(args) != 1 {
		return true, 1
	}
	if handler.check != nil && handler.check(args) != nil {
		return true, 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- handler.run(ctx) }()
	current := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	updates <- current
	for {
		select {
		case err := <-done:
			if err != nil {
				return true, 2
			}
			return false, 0
		case request, ok := <-changes:
			if ok && request.Cmd == svc.Interrogate {
				updates <- current
				continue
			}
			if ok && request.Cmd != svc.Stop && request.Cmd != svc.Shutdown {
				continue
			}
			cancel()
			current = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: 10000}
			updates <- current
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case err := <-done:
					if err != nil {
						return true, 2
					}
					return false, 0
				case <-ticker.C:
					current.CheckPoint++
					updates <- current
				case request, ok := <-changes:
					if !ok {
						changes = nil
						continue
					}
					if request.Cmd == svc.Interrogate {
						updates <- current
					}
				}
			}
		}
	}
}
