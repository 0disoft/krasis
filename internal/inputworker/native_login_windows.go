//go:build windows

package inputworker

import (
	"context"
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
	return startNativeChildClient(ctx, "--native-login-worker", handles, data[:])
}

// RunInstalledNativeLoginInput is a fixed SYSTEM/UIAccess child mode, bound to
// the active Winlogon grant and a genuine live service/console parent. It never
// authenticates from a profile hash or inherited numeric reference alone.
func RunInstalledNativeLoginInput(ctx context.Context, installation *Installation) error {
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
	if !validNativeParent(handles[1], installation, digest) || startup.Err() != nil || wininput.InitializePhysicalCoordinates() != nil {
		return ErrAuthority
	}
	authorize := func() error {
		if proof.check(false) != nil || !nativeParentAlive(handles[1]) {
			return ErrAuthority
		}
		return authorizeLoginProfile(binding.session, binding.profile)
	}
	if err := authorize(); err != nil {
		return err
	}
	input := wininput.NewInjectionPort()
	return inputdesktop.RunWinlogon(ctx, authorize, func(scope *inputdesktop.Scope) error {
		_, err := crashguard.ServeWithInput(os.Stdin, os.Stdout, input, PortableInputHandler(input, scope.Do))
		return err
	})
}
