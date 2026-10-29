//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/wininput"
	"golang.org/x/sys/windows"
)

func workerPipeName(pid uint32) string {
	return `\\.\pipe\Krasis.Input.Worker.v1.` + strconv.FormatUint(uint64(pid), 10)
}

// RunAgentLease transfers the authenticated pipe and agent-image pin only after
// its exact owned child acknowledges independent service authentication.
// Digests come from the protected installation record, never agent metadata.
func RunAgentLease(ctx context.Context, channel *Channel, lease Lease, agentDigest, serviceDigest [sha256.Size]byte) (report Report, result error) {
	return runInstalledAgentLease(ctx, channel, lease, agentDigest, serviceDigest, nil)
}

func runInstalledAgentLease(ctx context.Context, channel *Channel, lease Lease, agentDigest, serviceDigest [sha256.Size]byte, installation *Installation) (report Report, result error) {
	return runPreparedAgentLease(ctx, channel, lease, agentDigest, serviceDigest, installation, nil)
}

func runPreparedAgentLease(ctx context.Context, channel *Channel, lease Lease, agentDigest, serviceDigest [sha256.Size]byte, installation *Installation, candidate *preparedConsole, afterReady ...func()) (report Report, result error) {
	validationStarted := time.Now()
	startupStage := "validation"
	defer func() {
		if lease.Capture {
			result = sourceStartupFailure(startupStage, report, result)
		} else {
			result = inputStartupFailure(startupStage, result)
		}
	}()
	defer func() { result = validateSourceReportMode(lease.Capture, report, result) }()
	if candidate != nil {
		defer func() {
			if closeErr := candidate.close(); closeErr != nil {
				result = errors.Join(result, ErrRecovery, closeErr)
			}
		}()
	}
	if ctx == nil || channel == nil || channel.authority == nil || channel.authority.kind != authorityAgent || agentDigest == ([sha256.Size]byte{}) || serviceDigest == ([sha256.Size]byte{}) {
		return Report{}, ErrAuthority
	}
	if err := ctx.Err(); err != nil {
		return Report{}, errors.Join(ErrAuthority, err)
	}
	// Service-local diagnostics retain the native result. Ordinary authority
	// callers keep their sentinel-only contract; diagnostics export fixed fields.
	checkAgent := func(recovery bool) error { return channel.authority.checkResult(nil, recovery) }
	if err := checkAgent(false); err != nil {
		return Report{}, errors.Join(ErrAuthority, err)
	}
	defer channel.Close()
	agentPipe, ok := channel.stream.(*pipeConn)
	if !ok {
		return Report{}, ErrAuthority
	}
	data, err := encodeLease(lease, time.Now())
	if err != nil {
		return Report{}, ErrAuthority
	}
	prepared := candidate
	startupStage = "prepare"
	decision := candidate.claimDecision(ctx)
	if decision != handoff.PreparationUsed {
		if closeErr := candidate.close(); closeErr != nil {
			return Report{}, errors.Join(ErrRecovery, closeErr)
		}
		prepared, err = prepareLeaseConsole(ctx, channel.authority, serviceDigest, installation, lease)
		if err != nil {
			return Report{}, err
		}
	}
	worker, control, pid, profile := prepared.worker, prepared.control, prepared.pid, prepared.profile
	profile.ServiceReadiness = channel.serviceReadiness
	profile.PreparationDecision = decision
	if channel.preparationWaitMeasured {
		profile.RecordPreparationWait(channel.preparationWait)
	}
	defer func() {
		if closeErr := prepared.close(); closeErr != nil {
			result = errors.Join(result, ErrRecovery, closeErr)
		}
	}()
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	var actual uint32
	startupStage = "authentication"
	// Tag the original startup failure before deferred cleanup can add a
	// different operation. Capture keeps its independent source stage/report.
	failStartup := func(cause error) error {
		if lease.Capture {
			return cause
		}
		return inputStartupFailure(startupStage, cause)
	}
	if err := windows.GetNamedPipeClientProcessId(control.handle, &actual); err != nil {
		return Report{}, failStartup(errors.Join(ErrAuthority, err))
	}
	if actual != pid || windows.WTSGetActiveConsoleSessionId() != prepared.session {
		return Report{}, failStartup(ErrAuthority)
	}
	if err := checkAgent(false); err != nil {
		return Report{}, failStartup(errors.Join(ErrAuthority, err))
	}
	if err := control.SetDeadline(time.Now().Add(startupTimeout)); err != nil {
		return Report{}, failStartup(errors.Join(ErrAuthority, err))
	}
	stopStartup := context.AfterFunc(startup, func() { _ = control.Close() })
	defer stopStartup()
	profile.ServiceReadiness.Record(handoff.ServiceBootstrapValidation, time.Since(validationStarted))
	started := time.Now()
	// The worker cannot acknowledge before full service-image validation and
	// SCM/actual pipe-owner authentication. No agent handle is transferred yet.
	prepared.bootstrapBegun.Store(true)
	packet, err := bootstrapAfterAuthentication(startup, control, func() error {
		if err := windows.GetNamedPipeClientProcessId(control.handle, &actual); err != nil {
			return errors.Join(ErrAuthority, err)
		}
		if actual != pid || windows.WTSGetActiveConsoleSessionId() != prepared.session {
			return ErrAuthority
		}
		return checkAgent(false)
	}, func() ([bootstrapSize]byte, error) {
		startupStage = "transfer"
		if windows.WTSGetActiveConsoleSessionId() != prepared.session {
			return [bootstrapSize]byte{}, ErrAuthority
		}
		return transferWorkerBootstrap(startup, worker, channel.authority, agentPipe, agentDigest, serviceDigest, data, installation)
	})
	if err != nil {
		return Report{}, failStartup(errors.Join(ErrAuthority, err))
	}
	if !lease.Capture && !lease.Presentation {
		worker.expectNativeChild()
	}
	writeErr := writeFrame(control, packet[:])
	var childProfile *handoff.StartupProfile
	var readyErr error
	var finished bool
	if writeErr == nil {
		startupStage = "ready"
		if lease.Capture {
			childProfile, report, finished, readyErr = readSourceWorkerStartup(control)
		} else {
			childProfile, report, finished, readyErr = readInputWorkerStartup(control)
		}
	}
	profile.Record(handoff.ServiceBootstrapReady, time.Since(started))
	if finished {
		if !lease.Capture {
			// An early final report is not READY. Only its complete empty
			// recovery receipt can certify the native children; then join the
			// exact owned job before telling the agent this lease was rejected.
			stopStartup()
			if !errors.Is(readyErr, ErrRecovery) && !errors.Is(readyErr, ErrProtocol) {
				worker.confirmNativeRetirement()
				if closeErr := prepared.close(); closeErr != nil {
					return report, errors.Join(readyErr, closeErr, ErrRecovery)
				}
				readyErr = errors.Join(ErrStartupRetired, readyErr)
				if err := publishInputStartupRetired(ctx, channel.stream, report, readyErr); err != nil {
					return report, errors.Join(readyErr, err)
				}
			}
		}
		// No readiness is published. Native retirement reached the agent on
		// its own authenticated pipe; retain the independent service report.
		return report, observeMissingWorkerReport(readyErr, worker.reportExit)
	}
	if writeErr != nil || readyErr != nil || startup.Err() != nil {
		stopStartup()
		if lease.Capture && readyErr != nil {
			return report, observeMissingWorkerReport(readyErr, worker.reportExit)
		}
		return Report{}, failStartup(errors.Join(ErrAuthority, writeErr, readyErr, startup.Err()))
	}
	if !stopStartup() {
		return Report{}, failStartup(errors.Join(ErrAuthority, startup.Err()))
	}
	if err := control.SetDeadline(time.Time{}); err != nil {
		return Report{}, failStartup(errors.Join(ErrAuthority, err))
	}
	mergeWorkerStartup(&profile, childProfile)
	startupStage = "publish"
	if publishErr := publishStartupReady(ctx, channel, lease.ExpiresAt, profile); publishErr != nil {
		// The child is ready but no readiness receipt reached the agent. Still
		// request shutdown and consume recovery before killing the job.
		stopped, stop := context.WithCancel(context.Background())
		stop()
		report, result = supervise(stopped, control, func() error { return ErrAuthority }, worker.exited, lease.ExpiresAt)
		if !errors.Is(result, ErrRecovery) {
			worker.confirmNativeRetirement()
		}
		result = observeMissingWorkerReport(result, worker.reportExit)
		if !lease.Capture {
			publishErr = inputStartupFailure("publish", publishErr)
		}
		return report, errors.Join(publishErr, result)
	}
	startupStage = ""
	for _, notify := range afterReady {
		if notify != nil {
			notify()
		}
	}
	// Supervision grants no input authority; commands and recovery retain
	// their full checks. Do not stall stop/report consumption on WTS RPC.
	report, result = supervise(ctx, control, channel.authority.checkHeartbeat, worker.exited, lease.ExpiresAt)
	if !errors.Is(result, ErrRecovery) {
		worker.confirmNativeRetirement()
	}
	return report, observeMissingWorkerReport(result, worker.reportExit)
}

// RunConsoleInput is reachable only from the fixed SYSTEM/UIAccess worker
// mode. The protected installation supplies both digests. No command arguments
// may select a pipe handle, executable, PID, session or token source.
func RunConsoleInput(ctx context.Context, agentDigest, serviceDigest [sha256.Size]byte) error {
	return runConsoleInput(ctx, agentDigest, serviceDigest, handoff.StartupProfile{})
}

func runConsoleInput(ctx context.Context, agentDigest, serviceDigest [sha256.Size]byte, profile handoff.StartupProfile) error {
	return runInstalledConsoleInput(ctx, agentDigest, serviceDigest, profile, nil)
}

func runInstalledConsoleInput(ctx context.Context, agentDigest, serviceDigest [sha256.Size]byte, profile handoff.StartupProfile, installation *Installation) error {
	return runInstalledConsoleInputMode(ctx, agentDigest, serviceDigest, profile, installation, false)
}

func runInstalledConsoleInputMode(ctx context.Context, agentDigest, serviceDigest [sha256.Size]byte, profile handoff.StartupProfile, installation *Installation, prepareNative bool) (result error) {
	if ctx == nil || ctx.Err() != nil || agentDigest == ([sha256.Size]byte{}) || serviceDigest == ([sha256.Size]byte{}) {
		return ErrAuthority
	}
	started := time.Now()
	self, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		return ErrAuthority
	}
	allowed := validConsoleToken(self.token, windows.WTSGetActiveConsoleSessionId()) && self.integrity == 0x4000
	_ = self.close()
	profile.Record(handoff.WorkerIdentity, time.Since(started))
	if !allowed {
		return ErrAuthority
	}
	startup, stopStartup := context.WithTimeout(ctx, startupTimeout)
	defer stopStartup()
	control, err := openPipe(startup, workerPipeName(windows.GetCurrentProcessId()))
	if err != nil {
		return ErrAuthority
	}
	lifetime := &workerControlLifetime{stream: control}
	defer lifetime.Close()
	stopControl := sync.OnceValue(context.AfterFunc(startup, func() { _ = control.Close() }))
	defer stopControl()
	started = time.Now()
	proof, err := authenticateServiceInstalled(startup, uintptr(control.handle), serviceDigest, installation)
	profile.Record(handoff.WorkerServiceAuth, time.Since(started))
	if err != nil {
		return ErrAuthority
	}
	lifetime.proof = proof
	var native *preparedNativePair
	if prepareNative {
		if installation == nil {
			return ErrAuthority
		}
		// No native process exists during cancellable OS launch/connect.
		// The exact service owner must explicitly start unbound preparation.
		if err := awaitNativePreparation(control); err != nil {
			return err
		}
		native, err = prepareNativePair(startup)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, native.Close()) }()
	}
	if err := control.SetDeadline(time.Now().Add(startupTimeout)); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	if startup.Err() != nil || proof.check(false) != nil || writeFrame(control, workerAuthenticatedFrame[:]) != nil {
		return ErrAuthority
	}
	var packet [bootstrapSize]byte
	if prepareNative {
		packet, err = readPreparedBootstrap(startup, stopControl, control.readContext)
		if err != nil {
			return errors.Join(ErrProtocol, reportPreparationFailure(control, preparationBootstrapFailure, err))
		}
	} else {
		_, err = io.ReadFull(control, packet[:])
	}
	if err != nil {
		return ErrProtocol
	}
	if prepareNative && zeroBytes(packet[:]) {
		return retireUnboundNative(startup, control, stopControl, native.Close)
	}
	handle, imageHandle, err := decodeWorkerBootstrapHandles(packet, agentDigest)
	if err != nil {
		return ErrProtocol
	}
	pipe := &pipeConn{handle: windows.Handle(handle), closed: make(chan struct{}), closeDone: make(chan struct{})}
	pipeOwned := true
	defer func() {
		if pipeOwned {
			_ = pipe.Close()
		}
	}()
	image := os.NewFile(uintptr(imageHandle), "transferred-input-image")
	defer func() {
		if image != nil {
			_ = image.Close()
		}
	}()
	var leaseData [leaseSize]byte
	copy(leaseData[:], packet[bootstrapLeaseOffset:])
	lease, err := decodeLease(leaseData, time.Now())
	if err != nil || prepareNative && (lease.Capture || lease.Presentation) || startup.Err() != nil || proof.check(false) != nil {
		return ErrAuthority
	}
	started = time.Now()
	if installation != nil {
		if installation.adoptWorkerAgentImage(proof, image) != nil {
			return ErrAuthority
		}
		image = nil // installation owns the exact live pin until worker exit.
	}
	authority, err := authenticateAgentInstalled(uintptr(pipe.handle), agentDigest, installation)
	profile.Record(handoff.WorkerAgentAuth, time.Since(started))
	if err != nil {
		return ErrAuthority
	}
	channel := &Channel{stream: pipe, authority: authority}
	pipeOwned = false // Channel now retains the pipe through pending native queries.
	defer channel.Close()
	if wininput.InitializePhysicalCoordinates() != nil {
		return ErrAuthority
	}
	if startup.Err() != nil || !stopControl() {
		return ErrAuthority
	}
	if err := control.SetDeadline(time.Time{}); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	// All worker modes retain service proof queries separately from final reports.
	queryService := lifetime.captureChannel()
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	var requested atomic.Bool
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		var stop [1]byte
		if _, err := io.ReadFull(control, stop[:]); err == nil && stop[0] == 'S' {
			requested.Store(true)
		}
		cancel()
	}()
	defer func() {
		lifetime.interruptReader()
		<-readerDone
	}()
	if lease.Capture {
		var exit logindiag.SourceExit
		result := runSourceCapture(run, channel, queryService, lease, func() error {
			if err := control.SetWriteDeadline(time.Now().Add(startupTimeout)); err != nil {
				return errors.Join(ErrAuthority, err)
			}
			return writeStartupReady(control, profile)
		}, &exit)
		return finishWorkerReport(control, Report{Source: &exit}, result)
	}
	if lease.Presentation {
		desktopStarted := time.Now()
		result := runCursorPresentation(run, channel, queryService, lease, func() error {
			profile.Record(handoff.WorkerDesktopReady, time.Since(desktopStarted))
			if err := control.SetWriteDeadline(time.Now().Add(startupTimeout)); err != nil {
				return errors.Join(ErrAuthority, err)
			}
			return writeStartupReady(control, profile)
		})
		return finishWorkerReport(control, Report{}, result)
	}
	desktop := desktopRunner(inputdesktop.Run)
	if installation != nil {
		desktop = func(ctx context.Context, callback func(*inputdesktop.Scope) error) error {
			return inputdesktop.RunInteractive(ctx, func() error {
				// Recovery still checks the pinned installation/console but may
				// release journaled input after the agent has exited. Presses use
				// the stricter authority.check(false) in the serving executor.
				return authority.check(true)
			}, callback)
		}
	}
	var input application.InputPort = wininput.NewInjectionPort()
	var isolated *isolatedInput
	if installation != nil {
		isolated, err = startIsolatedInput(run, control, channel, queryService, lease, installation, &profile, native)
		if err != nil {
			// Failed startup published neither a journal nor input authority.
			// Preserve real pending-owner failures; clean rejection is not a
			// fabricated recovery failure. Close all read-only proof owners too.
			err = errors.Join(err, serviceRecoveryFailure(nil, channel.Close()), serviceRecoveryFailure(nil, queryService.Close()))
			return finishInputWorkerReport(control, Report{}, err, native.Close)
		}
		input = isolated
	}
	announceReady := func() error {
		return publishWorkerReady(run, queryService, control, lease.ExpiresAt, profile)
	}
	var report Report
	if isolated != nil && isolated.recoveryRetire != nil {
		report, result = serveIsolatedJournal(run, channel, isolated, lease, announceReady)
	} else {
		report, result = serveReadyProfileOnDesktop(run, channel, input, lease, announceReady, &profile, desktop)
	}
	if isolated != nil && !isolated.recovering {
		retired, retireErr := isolated.retireForRecovery()
		result = errors.Join(result, retireErr)
		if !retired {
			result = errors.Join(result, ErrRecovery)
		}
	}
	if isolated != nil {
		result = errors.Join(result, isolated.finishRecovery())
	}
	// A live startup proof cannot certify cleanup, even with an empty journal.
	result = errors.Join(result, serviceRecoveryFailure(nil, queryService.Close()))
	result = normalizeWorkerStop(requested.Load(), report, result)
	return finishInputWorkerReport(control, report, result, native.Close)
}

// No PID reopening: worker.process is the handle returned for the exact
// job-bound child. Failed/partial duplication is reaped by the owned job.
func transferWorkerBootstrap(ctx context.Context, worker *ConsoleWorker, authority *Authority, pipe *pipeConn, agent, service [sha256.Size]byte, lease [leaseSize]byte, installation *Installation) ([bootstrapSize]byte, error) {
	if ctx == nil || worker == nil || pipe == nil || authority == nil || authority.kind != authorityAgent {
		return [bootstrapSize]byte{}, ErrAuthority
	}
	if err := ctx.Err(); err != nil {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	if err := authority.checkResult(nil, false); err != nil {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	if installation != nil {
		a, s, err := installation.digests()
		if err != nil {
			return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
		}
		if a != agent || s != service {
			return [bootstrapSize]byte{}, ErrAuthority
		}
	}
	image, err := pinIdentityImage(installation, filepath.Join(root, "Krasis", "Input", "krasis-agent.exe"), agent)
	if err != nil {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	defer image.Close()
	worker.mu.Lock()
	defer worker.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	if worker.closed || worker.retiring || worker.process == 0 {
		return [bootstrapSize]byte{}, ErrAuthority
	}
	if err := authority.checkResult(nil, false); err != nil {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	state, err := windows.WaitForSingleObject(worker.process, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	var pipeHandle, imageHandle windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), pipe.handle, worker.process, &pipeHandle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	if err := windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(image.Fd()), worker.process, &imageHandle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	if err := ctx.Err(); err != nil {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	if err := authority.checkResult(nil, false); err != nil {
		return [bootstrapSize]byte{}, errors.Join(ErrAuthority, err)
	}
	return encodeWorkerBootstrap(uint64(pipeHandle), uint64(imageHandle), agent, lease)
}
