//go:build windows

package inputworker

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/0disoft/krasis/internal/loginagent"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// Private launch primitive for the authenticated restricted connection runner.
// A scheduling hash alone is never launch authority; fixed installation, SCM,
// live profile and child token checks are required independently.
func launchLoginConsole(ctx context.Context, installation *Installation, session uint32, profile [32]byte, prepare func(windows.Handle, uint32) error) (*ConsoleWorker, error) {
	if ctx == nil || ctx.Err() != nil || installation == nil || session == 0 || session == ^uint32(0) || profile == ([32]byte{}) || prepare == nil {
		return nil, ErrAuthority
	}
	_, digest, err := installation.digests()
	if err != nil || !fixedInstalledInputProcess(digest, false, installation) {
		return nil, ErrAuthority
	}
	managed, err := svc.IsWindowsService()
	if err != nil || !managed {
		return nil, ErrAuthority
	}
	authorized := func() bool {
		if ctx.Err() != nil || !fixedInstalledInputProcess(digest, false, installation) {
			return false
		}
		return authorizeLoginProfile(session, profile) == nil
	}
	return launchManagedConsoleMode(ctx, digest, prepare, installation, loginConsole, authorized)
}

func authorizeLoginProfile(session uint32, profile [32]byte) error {
	if session == 0 || session == ^uint32(0) || profile == ([32]byte{}) {
		return ErrAuthority
	}
	expected := loginagent.Console{Session: session, State: loginagent.BeforeLogin}
	console, err := loginagent.ReadConsole()
	if err != nil || console != expected {
		return ErrAuthority
	}
	now := time.Now().UTC()
	grant, present, err := ReadLoginGrant(now)
	if err != nil || !present {
		return ErrAuthority
	}
	selected, err := grant.Selection(now)
	if err != nil || !selected.Enabled || selected.Profile != profile {
		return ErrAuthority
	}
	if _, err := loginagent.ReadGrantConnection(grant, now); err != nil {
		return ErrAuthority
	}
	console, err = loginagent.ReadConsole()
	if err != nil || console != expected {
		return ErrAuthority
	}
	return nil
}

func loginPipeName(pid uint32) string {
	return `\\.\pipe\Krasis.Login.Worker.v1.` + strconv.FormatUint(uint64(pid), 10)
}

// The fixed child has no authority until both kernel identities and the live
// grant are checked and the final activation frame is accepted.
func launchLoginConnection(ctx context.Context, installation *Installation, session uint32, profile [32]byte) (result loginagent.Worker, resultErr error) {
	if ctx == nil || ctx.Err() != nil || installation == nil {
		return nil, ErrAuthority
	}
	binding, err := newLoginBinding(session, profile)
	if err != nil {
		return nil, err
	}
	var control *pipeConn
	var pid uint32
	child, err := launchLoginConsole(ctx, installation, session, profile, func(_ windows.Handle, actual uint32) error {
		pid = actual
		var err error
		control, err = makePipeACL(loginPipeName(pid), "D:P(A;;GA;;;SY)")
		return err
	})
	adopted := false
	activated := false
	defer func() {
		if !adopted {
			if activated {
				resultErr = errors.Join(resultErr, ErrRecovery)
			}
			if control != nil {
				resultErr = errors.Join(resultErr, control.Close())
			}
			if child != nil && child.closeWithin(1000) != nil {
				resultErr = errors.Join(resultErr, ErrRecovery)
			}
		}
	}()
	if err != nil {
		return nil, &loginRuntimeFailure{reason: "launch", cause: err}
	}
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if err := control.connect(startup); err != nil {
		return nil, &loginRuntimeFailure{reason: "pipe", cause: err}
	}
	if err := control.SetDeadline(time.Now().Add(startupTimeout)); err != nil {
		return nil, err
	}
	stopStartup := context.AfterFunc(startup, func() { _ = control.Close() })
	defer stopStartup()
	authenticate := func() error {
		var actual uint32
		if windows.GetNamedPipeClientProcessId(control.handle, &actual) != nil || actual != pid || child.exited() || authorizeLoginProfile(session, profile) != nil {
			return ErrAuthority
		}
		return nil
	}
	if err := loginParentBootstrap(startup, control, binding, authenticate, child.expectNativeChild); err != nil {
		return nil, &loginRuntimeFailure{reason: "bootstrap", cause: err}
	}
	activated = true
	if startup.Err() != nil || !stopStartup() {
		return nil, &loginRuntimeFailure{reason: "activation", cause: ErrAuthority}
	}
	if err := control.SetDeadline(time.Time{}); err != nil {
		return nil, ErrRecovery
	}
	result = startLoginControlWorker(ctx, control, binding, func() error { return child.closeWithin(1000) }, child.confirmNativeRetirement)
	adopted = true
	return result, nil
}

// RunInstalledLoginWorker is a distinct service-owned mode. A portable process,
// SYSTEM session-0 caller, or worker without the genuine SCM pipe cannot call
// run. The callback must join input and network cleanup before returning.
func RunInstalledLoginWorker(ctx context.Context, installation *Installation, run func(context.Context, uint32, [32]byte) error) (result error) {
	if run == nil {
		return ErrAuthority
	}
	return runInstalledLoginWorker(ctx, installation, func(lifetime context.Context, session uint32, profile [32]byte, _ func() error, _ *loginNativeOwner) error {
		return run(lifetime, session, profile)
	})
}

func runInstalledLoginWorker(ctx context.Context, installation *Installation, run func(context.Context, uint32, [32]byte, func() error, *loginNativeOwner) error) (result error) {
	if ctx == nil || ctx.Err() != nil || installation == nil || run == nil {
		return ErrAuthority
	}
	_, digest, err := installation.digests()
	if err != nil || !fixedInstalledInputProcess(digest, true, installation) {
		return ErrAuthority
	}
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	control, err := openPipe(startup, loginPipeName(windows.GetCurrentProcessId()))
	if err != nil {
		return ErrAuthority
	}
	defer func() { result = errors.Join(result, control.Close()) }()
	stopStartup := context.AfterFunc(startup, func() { _ = control.Close() })
	defer stopStartup()
	proof, err := authenticateServiceInstalled(startup, uintptr(control.handle), digest, installation)
	if err != nil {
		return ErrAuthority
	}
	defer func() { result = errors.Join(result, proof.Close()) }()
	if err := control.SetDeadline(time.Now().Add(startupTimeout)); err != nil {
		return ErrAuthority
	}
	binding, err := loginChildBootstrap(startup, control, func() error { return proof.check(false) }, func(b loginBinding) error {
		return authorizeLoginProfile(b.session, b.profile)
	})
	if err != nil {
		return err
	}
	if startup.Err() != nil || !stopStartup() {
		return ErrAuthority
	}
	if err := control.SetDeadline(time.Time{}); err != nil {
		return ErrRecovery
	}
	return serveLoginControl(ctx, control, binding, func() error { return proof.check(false) }, func(lifetime context.Context) error {
		return run(lifetime, binding.session, binding.profile, func() error { return proof.check(false) }, &loginNativeOwner{installation: installation, control: control, proof: proof, binding: binding})
	})
}
