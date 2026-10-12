//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/handoff"
	"golang.org/x/sys/windows"
)

const ServiceName = "KrasisInput"

// x/sys's token-class list stops at TokenLogonSid. This Win32 ABI value is
// stable and is defined in TOKEN_INFORMATION_CLASS in the Windows SDK.
const tokenIsAppContainer uint32 = 29

type processIdentity struct {
	handle                                     windows.Handle
	token                                      windows.Token
	session, integrity                         uint32
	uiAccess, restricted, appContainer, system bool
	image                                      string
}

// AuthenticateAgent uses only Windows pipe metadata to locate a client. The
// expected digest must come from administrator-owned installation metadata,
// never from a request, user profile, environment variable or local API payload.
// This function is intended for the SYSTEM service, not the ordinary agent.
func AuthenticateAgent(pipe uintptr, expected [sha256.Size]byte) (*Authority, error) {
	return authenticateAgentInstalled(pipe, expected, nil)
}

func authenticateAgentInstalled(pipe uintptr, expected [sha256.Size]byte, installation *Installation) (*Authority, error) {
	if pipe == 0 || expected == ([sha256.Size]byte{}) {
		return nil, ErrAuthority
	}
	var pid uint32
	if windows.GetNamedPipeClientProcessId(windows.Handle(pipe), &pid) != nil {
		return nil, ErrAuthority
	}
	identity, err := inspectProcess(pid)
	if err != nil {
		return nil, ErrAuthority
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = identity.close()
		}
	}()
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(identity.handle, &created, &exited, &kernel, &user) != nil {
		return nil, ErrAuthority
	}
	agentStarted := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	if agentStarted == 0 {
		return nil, ErrAuthority
	}
	session := windows.WTSGetActiveConsoleSessionId()
	root, rootErr := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if !validAgentIdentity(identity, session) || rootErr != nil || !strings.EqualFold(identity.image, filepath.Join(root, "Krasis", "Input", "krasis-agent.exe")) {
		return nil, ErrAuthority
	}
	var console windows.Token
	if windows.WTSQueryUserToken(session, &console) != nil {
		return nil, ErrAuthority
	}
	defer console.Close()
	clientUser, err := identity.token.GetTokenUser()
	consoleUser, consoleErr := console.GetTokenUser()
	if err != nil || consoleErr != nil || !clientUser.User.Sid.Equals(consoleUser.User.Sid) {
		return nil, ErrAuthority
	}
	logon, err := tokenLogon(identity.token)
	consoleLogon, consoleErr := tokenLogon(console)
	if err != nil || consoleErr != nil || logon != consoleLogon {
		return nil, ErrAuthority
	}
	image, err := pinIdentityImage(installation, identity.image, expected)
	if err != nil {
		return nil, ErrAuthority
	}
	consoleStillCurrent := func() error {
		if windows.WTSGetActiveConsoleSessionId() != session {
			return ErrAuthority
		}
		var current windows.Token
		if windows.WTSQueryUserToken(session, &current) != nil {
			return ErrAuthority
		}
		defer current.Close()
		currentLogon, err := tokenLogon(current)
		if err != nil || currentLogon != logon {
			return ErrAuthority
		}
		return nil
	}
	accepted = true
	return &Authority{
		kind:         authorityAgent,
		agentStarted: agentStarted,
		checkInput: func() error {
			state, err := windows.WaitForSingleObject(identity.handle, 0)
			if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
				return ErrAuthority
			}
			return consoleStillCurrent()
		},
		// Agent exit must not block key-up. A different console logon must.
		checkRecovery: consoleStillCurrent,
		closeHandles: func() error {
			imageErr := image.Close()
			identityErr := identity.close()
			if imageErr != nil || identityErr != nil {
				return ErrAuthority
			}
			return nil
		},
	}, nil
}

// AuthenticateService verifies the fixed LocalSystem service configuration,
// actual kernel pipe owner and protected image. SCM owns the process identity
// of this administrator-protected LocalSystem own-process service; ordinary
// agents do not need access to its process object or token.
// The caller pins the digest from an independently validated update package.
func AuthenticateService(pipe uintptr, expected [sha256.Size]byte) (*Authority, error) {
	return authenticateServiceObserved(context.Background(), pipe, expected)
}

func authenticateServiceObserved(ctx context.Context, pipe uintptr, expected [sha256.Size]byte) (*Authority, error) {
	return authenticateServiceInstalled(ctx, pipe, expected, nil)
}

func authenticateServiceInstalled(ctx context.Context, pipe uintptr, expected [sha256.Size]byte, installation *Installation) (*Authority, error) {
	if pipe == 0 || expected == ([sha256.Size]byte{}) {
		return nil, ErrAuthority
	}
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(windows.Handle(pipe), &pid); err != nil {
		return nil, connectionFailed("service_pid", err)
	}
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, connectionFailed("service_manager", err)
	}
	defer windows.CloseServiceHandle(manager)
	name, err := windows.UTF16PtrFromString(ServiceName)
	if err != nil {
		return nil, ErrAuthority
	}
	service, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG|windows.READ_CONTROL)
	if err != nil {
		return nil, connectionFailed("service_open", err)
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = windows.CloseServiceHandle(service)
		}
	}()
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return nil, ErrAuthority
	}
	path := filepath.Join(root, "Krasis", "Input", "krasis-input.exe")
	if err := validateInstallService(service, windows.EscapeArg(path)+" --service"); err != nil {
		return nil, connectionFailed("service_config", err)
	}
	checkObserved := serviceProcessBindingObserved(pid, func(ctx context.Context) (uint32, windows.SERVICE_STATUS_PROCESS, error) {
		return readServicePipeProcessObserved(ctx, windows.Handle(pipe), service)
	})
	check := func() error { return checkObserved(nil) }
	if err := check(); err != nil {
		return nil, err
	}
	started := time.Now()
	image, err := pinIdentityImage(installation, path, expected)
	handoff.Record(ctx, handoff.ServiceImagePin, time.Since(started))
	if err != nil {
		return nil, connectionFailed("service_image", err)
	}
	if protectedHandle(windows.Handle(image.Fd())) != nil {
		_ = image.Close()
		return nil, connectionFailed("service_image_acl", ErrAuthority)
	}
	// Recheck after pinning the image to reject a stop/restart during setup.
	if err := check(); err != nil {
		_ = image.Close()
		return nil, err
	}
	accepted = true
	return &Authority{kind: authorityService, checkInput: check, checkInputObserved: checkObserved, checkRecovery: check, closeHandles: func() error {
		imageErr := image.Close()
		serviceErr := windows.CloseServiceHandle(service)
		if imageErr != nil || serviceErr != nil {
			return ErrAuthority
		}
		return nil
	}}, nil
}

func validAgentIdentity(identity *processIdentity, session uint32) bool {
	return identity != nil && session != 0 && session != 0xffffffff && identity.session == session &&
		!identity.system && !identity.restricted && !identity.appContainer && identity.integrity >= 0x2000 && identity.integrity <= 0x3000
}

func inspectProcess(pid uint32) (*processIdentity, error) {
	if pid == 0 {
		return nil, ErrAuthority
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, ErrAuthority
	}
	identity := &processIdentity{handle: handle}
	accepted := false
	defer func() {
		if !accepted {
			_ = identity.close()
		}
	}()
	actual, err := windows.GetProcessId(handle)
	if err != nil || actual != pid || windows.ProcessIdToSessionId(pid, &identity.session) != nil {
		return nil, ErrAuthority
	}
	if windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &identity.token) != nil {
		return nil, ErrAuthority
	}
	user, err := identity.token.GetTokenUser()
	if err != nil {
		return nil, ErrAuthority
	}
	identity.system = user.User.Sid.IsWellKnown(windows.WinLocalSystemSid)
	var tokenSession uint32
	if tokenUint32(identity.token, windows.TokenSessionId, &tokenSession) != nil || tokenSession != identity.session {
		return nil, ErrAuthority
	}
	var value uint32
	if tokenUint32(identity.token, windows.TokenUIAccess, &value) != nil {
		return nil, ErrAuthority
	}
	identity.uiAccess = value != 0
	if tokenUint32(identity.token, tokenIsAppContainer, &value) != nil {
		return nil, ErrAuthority
	}
	identity.appContainer = value != 0
	identity.restricted, err = identity.token.IsRestricted()
	if err != nil {
		return nil, ErrAuthority
	}
	var length uint32
	err = windows.GetTokenInformation(identity.token, windows.TokenIntegrityLevel, nil, 0, &length)
	if err != windows.ERROR_INSUFFICIENT_BUFFER || length < uint32(unsafe.Sizeof(windows.Tokenmandatorylabel{})) || length > 4096 {
		return nil, ErrAuthority
	}
	buffer := make([]byte, length)
	if windows.GetTokenInformation(identity.token, windows.TokenIntegrityLevel, &buffer[0], length, &length) != nil {
		return nil, ErrAuthority
	}
	identity.integrity, err = integrityRID(buffer)
	if err != nil {
		return nil, ErrAuthority
	}
	var image [32768]uint16
	size := uint32(len(image))
	if windows.QueryFullProcessImageName(handle, 0, &image[0], &size) != nil || size == 0 || size >= uint32(len(image)) {
		return nil, ErrAuthority
	}
	identity.image = windows.UTF16ToString(image[:size])
	accepted = true
	return identity, nil
}

// Parse the kernel-filled SID in the caller-owned buffer without converting a
// returned uintptr back into a Go pointer. GetSidSubAuthority's wrapper does
// that conversion and fails checkptr when the SID lives in a Go allocation.
func integrityRID(buffer []byte) (uint32, error) {
	if len(buffer) < int(unsafe.Sizeof(windows.Tokenmandatorylabel{})) {
		return 0, ErrAuthority
	}
	var address uintptr
	if unsafe.Sizeof(uintptr(0)) == 8 {
		address = uintptr(binary.LittleEndian.Uint64(buffer[:8]))
	} else {
		address = uintptr(binary.LittleEndian.Uint32(buffer[:4]))
	}
	base := uintptr(unsafe.Pointer(&buffer[0]))
	if address < base {
		return 0, ErrAuthority
	}
	offset := address - base
	if offset < unsafe.Sizeof(windows.Tokenmandatorylabel{}) || offset > uintptr(len(buffer)) || uintptr(len(buffer))-offset < 12 {
		return 0, ErrAuthority
	}
	sid := buffer[offset : offset+12]
	// A mandatory-label SID is exactly S-1-16-RID; do not accept an arbitrary SID.
	if sid[0] != 1 || sid[1] != 1 || string(sid[2:8]) != "\x00\x00\x00\x00\x00\x10" {
		return 0, ErrAuthority
	}
	return binary.LittleEndian.Uint32(sid[8:12]), nil
}

func (identity *processIdentity) close() error {
	if identity == nil {
		return nil
	}
	var err error
	if identity.token != 0 {
		err = identity.token.Close()
		identity.token = 0
	}
	if identity.handle != 0 {
		if closeErr := windows.CloseHandle(identity.handle); closeErr != nil {
			err = closeErr
		}
		identity.handle = 0
	}
	return err
}

func tokenUint32(token windows.Token, class uint32, value *uint32) error {
	var returned uint32
	err := windows.GetTokenInformation(token, class, (*byte)(unsafe.Pointer(value)), 4, &returned)
	if err != nil || returned != 4 {
		return ErrAuthority
	}
	return nil
}

type tokenStatistics struct {
	TokenID, AuthenticationID                                    windows.LUID
	ExpirationTime                                               int64
	TokenType, ImpersonationLevel                                uint32
	DynamicCharged, DynamicAvailable, GroupCount, PrivilegeCount uint32
	ModifiedID                                                   windows.LUID
}

func tokenLogon(token windows.Token) (windows.LUID, error) {
	var stats tokenStatistics
	var returned uint32
	err := windows.GetTokenInformation(token, windows.TokenStatistics, (*byte)(unsafe.Pointer(&stats)), uint32(unsafe.Sizeof(stats)), &returned)
	if err != nil || returned != uint32(unsafe.Sizeof(stats)) {
		return windows.LUID{}, ErrAuthority
	}
	return stats.AuthenticationID, nil
}

// Runtime callers additionally require an administrator-owned fixed path. A
// digest read through a user-writable pathname alone cannot prove which image
// was loaded, because rename/replacement races can decouple those identities.
func lockImage(path string, expected [sha256.Size]byte) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, ErrAuthority
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, ErrAuthority
	}
	file := os.NewFile(uintptr(handle), "verified-input-image")
	accepted := false
	defer func() {
		if !accepted {
			_ = file.Close()
		}
	}()
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		return nil, ErrAuthority
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if size == 0 || size > 64<<20 {
		return nil, ErrAuthority
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, int64(size)+1))
	if err != nil || n != int64(size) || subtle.ConstantTimeCompare(hash.Sum(nil), expected[:]) != 1 {
		return nil, ErrAuthority
	}
	accepted = true
	return file, nil
}
