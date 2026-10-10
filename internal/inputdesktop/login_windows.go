//go:build windows

package inputdesktop

import (
	"context"
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RunWinlogon is a separate fixed target for an authenticated service child.
// authorize must verify the protected installation, genuine service channel
// and live machine grant. It is not a general desktop selector or elevation
// mechanism. Run keeps its strict Default-only policy.
func RunWinlogon(ctx context.Context, authorize func() error, callback func(*Scope) error) error {
	if ctx == nil || authorize == nil || callback == nil {
		return ErrInvalid
	}
	return run(ctx, func(scope *Scope) error {
		if err := authorize(); err != nil {
			return errors.Join(ErrUnavailable, err)
		}
		return callback(scope)
	}, func() (binding, error) {
		if err := authorize(); err != nil {
			return nil, errors.Join(ErrUnavailable, err)
		}
		if !privilegedLoginToken() {
			return nil, ErrUnavailable
		}
		return openTargetDesktop(winlogonTarget)
	}, currentThreadID, runtime.LockOSThread, runtime.UnlockOSThread)
}

// Inspect the actual process token on the locked thread, and reject a thread
// impersonation token rather than assuming the process's privileges apply.
// No rights or token attributes are changed here.
func privilegedLoginToken() (allowed bool) {
	var thread windows.Token
	if err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &thread); err != windows.ERROR_NO_TOKEN {
		if err == nil {
			_ = thread.Close()
		}
		return false
	}
	var token windows.Token
	if windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token) != nil {
		return false
	}
	defer func() {
		if token.Close() != nil {
			allowed = false
		}
	}()
	user, err := token.GetTokenUser()
	if err != nil || !user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		return false
	}
	session := windows.WTSGetActiveConsoleSessionId()
	if session == 0 || session == ^uint32(0) {
		return false
	}
	for _, field := range []struct {
		class uint32
		want  uint32
	}{
		{windows.TokenSessionId, session}, {windows.TokenUIAccess, 1}, {29, 0}, // TokenIsAppContainer
	} {
		var actual, size uint32
		if windows.GetTokenInformation(token, field.class, (*byte)(unsafe.Pointer(&actual)), 4, &size) != nil || size != 4 || actual != field.want {
			return false
		}
	}
	restricted, err := token.IsRestricted()
	if err != nil || restricted {
		return false
	}
	var size uint32
	if windows.GetTokenInformation(token, windows.TokenIntegrityLevel, nil, 0, &size) != windows.ERROR_INSUFFICIENT_BUFFER || size < uint32(unsafe.Sizeof(windows.Tokenmandatorylabel{})) || size > 4096 {
		return false
	}
	buffer := make([]byte, size)
	if windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &buffer[0], size, &size) != nil || size > uint32(len(buffer)) {
		return false
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buffer[0]))
	sid := label.Label.Sid
	return sid != nil && sid.IsValid() && sid.SubAuthorityCount() > 0 && sid.SubAuthority(uint32(sid.SubAuthorityCount()-1)) == 0x4000
}
