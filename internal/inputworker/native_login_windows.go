//go:build windows

package inputworker

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
	"golang.org/x/sys/windows"
)

// A capability constructed only after the real SCM login control handshake and
// grant activation. No ordinary agent or local API can construct this owner.
type loginNativeOwner struct {
	installation *Installation
	control      *pipeConn
	proof        *Authority
	binding      loginBinding
}

func (owner *loginNativeOwner) start(ctx context.Context) (*isolatedInput, error) {
	if ctx == nil || ctx.Err() != nil || owner == nil || owner.control == nil || owner.proof == nil || owner.proof.kind != authorityService {
		return nil, ErrAuthority
	}
	_, digest, err := owner.installation.digests()
	if err != nil || owner.proof.check(false) != nil || !fixedInstalledInputProcess(digest, true, owner.installation) || authorizeLoginProfile(owner.binding.session, owner.binding.profile) != nil {
		return nil, ErrAuthority
	}
	var handles []windows.Handle
	defer func() {
		for _, handle := range handles {
			_ = windows.CloseHandle(handle)
		}
	}()
	for _, source := range []windows.Handle{owner.control.handle, windows.CurrentProcess()} {
		var handle windows.Handle
		access, flags := uint32(0), uint32(windows.DUPLICATE_SAME_ACCESS)
		if source == windows.CurrentProcess() {
			access, flags = windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, 0
		}
		if windows.DuplicateHandle(windows.CurrentProcess(), source, windows.CurrentProcess(), &handle, access, true, flags) != nil {
			return nil, ErrAuthority
		}
		handles = append(handles, handle)
	}
	data, err := encodeNativeLoginBootstrap(owner.binding, uint64(handles[0]), uint64(handles[1]))
	if err != nil {
		return nil, err
	}
	input, err := startNativeChildClient(ctx, "--native-login-worker", handles, data[:])
	if err != nil {
		return nil, err
	}
	release, err := startNativeChildClient(ctx, "--native-login-release-worker", handles, data[:])
	if err != nil {
		retired, closeErr := input.retire()
		if !retired {
			closeErr = errors.Join(closeErr, ErrRecovery)
		}
		return nil, errors.Join(err, closeErr)
	}
	input.recovery, input.recoveryRetire = release.Client, release.retire
	return input, nil
}

// RunInstalledNativeLoginInput is a fixed SYSTEM/UIAccess child mode, bound to
// the active Winlogon grant and a genuine live service/console parent. It never
// authenticates from a profile hash or inherited numeric reference alone.
func RunInstalledNativeLoginInput(ctx context.Context, installation *Installation) error {
	return runInstalledNativeLoginInput(ctx, installation, false)
}

func RunInstalledNativeLoginRelease(ctx context.Context, installation *Installation) error {
	return runInstalledNativeLoginInput(ctx, installation, true)
}

func runInstalledNativeLoginInput(ctx context.Context, installation *Installation, releaseOnly bool) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrAuthority
	}
	_, digest, err := installation.digests()
	if err != nil || !fixedInstalledInputProcess(digest, true, installation) {
		return ErrAuthority
	}
	var data [nativeLoginBootstrapSize]byte
	if _, err := io.ReadFull(os.Stdin, data[:]); err != nil {
		return ErrAuthority
	}
	binding, values, err := decodeNativeLoginBootstrap(data)
	if err != nil {
		return err
	}
	handles, err := adoptNativeReferences(values, 2)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handles[0])
	defer windows.CloseHandle(handles[1])
	startup, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	proof, err := authenticateServiceInstalled(startup, uintptr(handles[0]), digest, installation)
	if err != nil {
		return err
	}
	defer proof.Close()
	serviceCheck := func() error { return proof.check(false) }
	if releaseOnly {
		handle, err := pinNativeRecoveryService(handles[0], proof)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(handle)
		serviceCheck = func() error { return checkNativeRecoveryService(handle, proof) }
	}
	if !validNativeParent(handles[1], installation, digest) || startup.Err() != nil || wininput.InitializePhysicalCoordinates() != nil {
		return ErrAuthority
	}
	authorize := func() error {
		if serviceCheck() != nil || !nativeParentAlive(handles[1]) {
			return ErrAuthority
		}
		return authorizeLoginProfile(binding.session, binding.profile)
	}
	if err := authorize(); err != nil {
		return err
	}
	input := wininput.NewInjectionPort()
	return inputdesktop.RunWinlogon(ctx, authorize, func(scope *inputdesktop.Scope) error {
		execute := func(ctx context.Context, work func() error) error {
			if err := authorize(); err != nil {
				return err
			}
			return scope.Do(ctx, work)
		}
		if releaseOnly {
			_, err := crashguard.ServeWithInput(os.Stdin, os.Stdout, deniedGuardRelease{}, PortableReleaseHandler(input, execute))
			return err
		}
		_, err := crashguard.ServeWithInput(os.Stdin, os.Stdout, input, PortableInputHandler(input, execute))
		return err
	})
}
