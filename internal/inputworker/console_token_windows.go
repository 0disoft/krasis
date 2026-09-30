//go:build windows

package inputworker

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// launchPrivileges enables only a duplicate thread impersonation token. The
// service process token and other threads retain their original privileges.
func launchPrivileges() (func() error, error) {
	var existing windows.Token
	if err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &existing); err != windows.ERROR_NO_TOKEN {
		if err == nil {
			_ = existing.Close()
		}
		return nil, ErrAuthority
	}
	self, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		return nil, ErrAuthority
	}
	defer self.close()
	if !self.system || self.session != 0 || self.integrity != 0x4000 || self.restricted || self.appContainer {
		return nil, ErrAuthority
	}
	var source windows.Token
	if windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE, &source) != nil {
		return nil, ErrAuthority
	}
	defer source.Close()
	var thread windows.Token
	if windows.DuplicateTokenEx(source, windows.TOKEN_QUERY|windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_IMPERSONATE, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &thread) != nil {
		return nil, ErrAuthority
	}
	defer thread.Close()
	for _, name := range []string{"SeTcbPrivilege", "SeDebugPrivilege", "SeAssignPrimaryTokenPrivilege", "SeIncreaseQuotaPrivilege"} {
		if enablePrivilege(thread, name) != nil {
			return nil, ErrAuthority
		}
	}
	if windows.SetThreadToken(nil, thread) != nil {
		return nil, ErrAuthority
	}
	return windows.RevertToSelf, nil
}

func enablePrivilege(token windows.Token, name string) error {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return ErrAuthority
	}
	privileges := windows.Tokenprivileges{PrivilegeCount: 1}
	if windows.LookupPrivilegeValue(nil, ptr, &privileges.Privileges[0].Luid) != nil {
		return ErrAuthority
	}
	privileges.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED
	if windows.AdjustTokenPrivileges(token, false, &privileges, 0, nil, nil) != nil || !privilegeEnabled(token, privileges.Privileges[0].Luid) {
		return ErrAuthority
	}
	return nil
}

func privilegeEnabled(token windows.Token, luid windows.LUID) bool {
	var length uint32
	if windows.GetTokenInformation(token, windows.TokenPrivileges, nil, 0, &length) != windows.ERROR_INSUFFICIENT_BUFFER || length < 4 || length > 4096 {
		return false
	}
	buffer := make([]byte, length)
	if windows.GetTokenInformation(token, windows.TokenPrivileges, &buffer[0], length, &length) != nil || length > uint32(len(buffer)) {
		return false
	}
	return containsEnabledPrivilege(buffer[:length], luid)
}

func containsEnabledPrivilege(buffer []byte, luid windows.LUID) bool {
	if len(buffer) < 4 {
		return false
	}
	count := binary.LittleEndian.Uint32(buffer[:4])
	if uint64(count)*12+4 != uint64(len(buffer)) {
		return false
	}
	for i := uint32(0); i < count; i++ {
		entry := buffer[4+i*12 : 4+(i+1)*12]
		if binary.LittleEndian.Uint32(entry[:4]) == luid.LowPart && int32(binary.LittleEndian.Uint32(entry[4:8])) == luid.HighPart {
			return binary.LittleEndian.Uint32(entry[8:12])&windows.SE_PRIVILEGE_ENABLED != 0
		}
	}
	return false
}

func consoleToken(session uint32) (windows.Token, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, ErrAuthority
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	var pid uint32
	for err == nil {
		var actual uint32
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "winlogon.exe") && windows.ProcessIdToSessionId(entry.ProcessID, &actual) == nil && actual == session {
			if pid != 0 {
				return 0, ErrAuthority
			}
			pid = entry.ProcessID
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if err != windows.ERROR_NO_MORE_FILES || pid == 0 {
		return 0, ErrAuthority
	}
	identity, err := inspectProcess(pid)
	if err != nil {
		return 0, ErrAuthority
	}
	defer identity.close()
	system, err := windows.GetSystemDirectory()
	if err != nil || !strings.EqualFold(identity.image, filepath.Join(system, "winlogon.exe")) ||
		!identity.system || identity.integrity != 0x4000 || identity.session != session || identity.restricted || identity.appContainer || windows.WTSGetActiveConsoleSessionId() != session {
		return 0, ErrAuthority
	}
	var source windows.Token
	if windows.OpenProcessToken(identity.handle, windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE, &source) != nil {
		return 0, ErrAuthority
	}
	defer source.Close()
	var token windows.Token
	if windows.DuplicateTokenEx(source, windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_ADJUST_PRIVILEGES, nil, windows.SecurityImpersonation, windows.TokenPrimary, &token) != nil {
		return 0, ErrAuthority
	}
	var enabled uint32 = 1
	// The child re-authenticates the transferred agent against the console
	// logon using WTSQueryUserToken, which requires SYSTEM + enabled SeTcb.
	if enablePrivilege(token, "SeTcbPrivilege") != nil || windows.SetTokenInformation(token, windows.TokenUIAccess, (*byte)(unsafe.Pointer(&enabled)), 4) != nil || !validConsoleToken(token, session) {
		_ = token.Close()
		return 0, ErrAuthority
	}
	return token, nil
}

func validConsoleToken(token windows.Token, session uint32) bool {
	user, err := token.GetTokenUser()
	if err != nil || !user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		return false
	}
	var actual, uiAccess, appContainer uint32
	if tokenUint32(token, windows.TokenSessionId, &actual) != nil || actual != session ||
		tokenUint32(token, windows.TokenUIAccess, &uiAccess) != nil || uiAccess != 1 ||
		tokenUint32(token, tokenIsAppContainer, &appContainer) != nil || appContainer != 0 {
		return false
	}
	restricted, err := token.IsRestricted()
	return err == nil && !restricted
}
