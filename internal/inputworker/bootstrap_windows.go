//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/wininput"
	"golang.org/x/sys/windows"
)

func workerPipeName(pid uint32) string {
	return `\\.\pipe\Krasis.Input.Worker.v1.` + strconv.FormatUint(uint64(pid), 10)
}

// RunAgentLease transfers only the authenticated pipe to the child it creates.
// Digests come from the protected installation record, never agent metadata.
func RunAgentLease(ctx context.Context, channel *Channel, lease Lease, agentDigest, serviceDigest [sha256.Size]byte) (report Report, result error) {
	if ctx == nil || ctx.Err() != nil || channel == nil || channel.authority == nil || channel.authority.kind != authorityAgent || channel.authority.check(false) != nil || agentDigest == ([sha256.Size]byte{}) || serviceDigest == ([sha256.Size]byte{}) {
		return Report{}, ErrAuthority
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
	var control *pipeConn
	defer func() {
		if control != nil {
			_ = control.Close()
		}
	}()
	var transferred windows.Handle
	var pid uint32
	var profile handoff.StartupProfile
	started := time.Now()
	worker, err := launchManagedConsole(ctx, channel.authority, serviceDigest, func(process windows.Handle, childID uint32) error {
		pid = childID
		var err error
		control, err = makePipeACL(workerPipeName(pid), "D:P(A;;GA;;;SY)")
		if err != nil {
			return ErrAuthority
		}
		if windows.DuplicateHandle(windows.CurrentProcess(), agentPipe.handle, process, &transferred, 0, false, windows.DUPLICATE_SAME_ACCESS) != nil {
			return ErrAuthority
		}
		return nil
	})
	profile.Record(handoff.ServiceLaunch, time.Since(started))
	if err != nil {
		return Report{}, ErrAuthority
	}
	defer func() {
		if worker.Close() != nil {
			result = errors.Join(result, ErrRecovery)
		}
	}()
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	started = time.Now()
	connectErr := control.connect(startup)
	profile.Record(handoff.ServiceChildConnect, time.Since(started))
	if connectErr != nil {
		return Report{}, ErrAuthority
	}
	var actual uint32
	if windows.GetNamedPipeClientProcessId(control.handle, &actual) != nil || actual != pid || channel.authority.check(false) != nil {
		return Report{}, ErrAuthority
	}
	var packet [bootstrapSize]byte
	copy(packet[:4], "KRB1")
	packet[4] = 1
	binary.BigEndian.PutUint64(packet[8:16], uint64(transferred))
	copy(packet[16:], data[:])
	_ = control.SetDeadline(time.Now().Add(startupTimeout))
	stopStartup := context.AfterFunc(startup, func() { _ = control.Close() })
	started = time.Now()
	writeErr := writeFrame(control, packet[:])
	var childProfile *handoff.StartupProfile
	var readyErr error
	if writeErr == nil {
		childProfile, readyErr = readStartupReady(control)
	}
	profile.Record(handoff.ServiceBootstrapReady, time.Since(started))
	if writeErr != nil || readyErr != nil || startup.Err() != nil {
		stopStartup()
		return Report{}, ErrAuthority
	}
	if !stopStartup() {
		return Report{}, ErrAuthority
	}
	_ = control.SetDeadline(time.Time{})
	mergeWorkerStartup(&profile, childProfile)
	if channel.authority.check(false) != nil || channel.stream.SetWriteDeadline(time.Now().Add(time.Second)) != nil || writeStartupReady(channel.stream, profile) != nil {
		// The child is ready but no readiness receipt reached the agent. Still
		// request shutdown and consume recovery before killing the job.
		stopped, stop := context.WithCancel(context.Background())
		stop()
		return supervise(stopped, control, func() error { return ErrAuthority }, worker.exited, lease.ExpiresAt)
	}
	_ = channel.stream.SetWriteDeadline(time.Time{})
	return supervise(ctx, control, func() error { return channel.authority.check(false) }, worker.exited, lease.ExpiresAt)
}

// RunConsoleInput is reachable only from the fixed SYSTEM/UIAccess worker
// mode. The protected installation supplies both digests. No command arguments
// may select a pipe handle, executable, PID, session or token source.
func RunConsoleInput(ctx context.Context, agentDigest, serviceDigest [sha256.Size]byte) error {
	return runConsoleInput(ctx, agentDigest, serviceDigest, handoff.StartupProfile{})
}

func runConsoleInput(ctx context.Context, agentDigest, serviceDigest [sha256.Size]byte, profile handoff.StartupProfile) error {
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
	defer control.Close()
	started = time.Now()
	proof, err := AuthenticateService(uintptr(control.handle), serviceDigest)
	profile.Record(handoff.WorkerServiceAuth, time.Since(started))
	if err != nil {
		return ErrAuthority
	}
	defer proof.Close()
	_ = control.SetReadDeadline(time.Now().Add(startupTimeout))
	var packet [bootstrapSize]byte
	if _, err := io.ReadFull(control, packet[:]); err != nil || string(packet[:4]) != "KRB1" || packet[4] != 1 || !zeroBytes(packet[5:8]) {
		return ErrProtocol
	}
	handle := binary.BigEndian.Uint64(packet[8:16])
	if handle == 0 || uint64(uintptr(handle)) != handle {
		return ErrProtocol
	}
	pipe := &pipeConn{handle: windows.Handle(handle), closed: make(chan struct{}), closeDone: make(chan struct{})}
	defer pipe.Close()
	var leaseData [leaseSize]byte
	copy(leaseData[:], packet[16:])
	lease, err := decodeLease(leaseData, time.Now())
	if err != nil || proof.check(false) != nil {
		return ErrAuthority
	}
	started = time.Now()
	authority, err := AuthenticateAgent(uintptr(pipe.handle), agentDigest)
	profile.Record(handoff.WorkerAgentAuth, time.Since(started))
	if err != nil {
		return ErrAuthority
	}
	channel := &Channel{stream: pipe, authority: authority}
	defer channel.Close()
	if wininput.InitializePhysicalCoordinates() != nil {
		return ErrAuthority
	}
	_ = control.SetReadDeadline(time.Time{})
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
	defer func() { _ = control.Close(); <-readerDone }()
	report, result := serveReadyProfile(run, channel, wininput.NewInputAdapter(), lease, func() error {
		if proof.check(false) != nil {
			return ErrAuthority
		}
		_ = control.SetWriteDeadline(time.Now().Add(startupTimeout))
		return writeStartupReady(control, profile)
	}, &profile)
	if requested.Load() && errors.Is(result, context.Canceled) && report.Recovery.Remaining == 0 {
		result = nil
	}
	_ = control.SetWriteDeadline(time.Now().Add(time.Second))
	if writeReport(control, report, result) != nil {
		return ErrRecovery
	}
	return result
}
