//go:build windows

package peercredentials

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const (
	seFileObject                     = 1
	daclSecurityInformation          = 0x00000004
	protectedDACLsecurityInformation = 0x80000000
	seDACLProtected                  = 0x1000
	sddlRevision1                    = 1
	fileAttributeReparsePoint        = 0x00000400
	invalidFileAttributes            = 0xffffffff
	accessAllowedACEType             = 0
	fileAllAccess                    = 0x001f01ff
	objectInheritACE                 = 0x01
	containerInheritACE              = 0x02
)

type windowsACL struct {
	Revision byte
	_        byte
	Size     uint16
	ACECount uint16
	_        uint16
}

type windowsACEHeader struct {
	Type  byte
	Flags byte
	Size  uint16
}

type windowsAccessAllowedACE struct {
	Header   windowsACEHeader
	Mask     uint32
	SIDStart uint32
}

var (
	advapi32                                                 = syscall.NewLazyDLL("advapi32.dll")
	kernel32                                                 = syscall.NewLazyDLL("kernel32.dll")
	procConvertStringSecurityDescriptorToSecurityDescriptorW = advapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	procGetSecurityDescriptorDACL                            = advapi32.NewProc("GetSecurityDescriptorDacl")
	procGetSecurityDescriptorControl                         = advapi32.NewProc("GetSecurityDescriptorControl")
	procSetNamedSecurityInfoW                                = advapi32.NewProc("SetNamedSecurityInfoW")
	procGetNamedSecurityInfoW                                = advapi32.NewProc("GetNamedSecurityInfoW")
	procSetSecurityInfo                                      = advapi32.NewProc("SetSecurityInfo")
	procGetSecurityInfo                                      = advapi32.NewProc("GetSecurityInfo")
	procGetACE                                               = advapi32.NewProc("GetAce")
	procGetFileAttributesW                                   = kernel32.NewProc("GetFileAttributesW")
)

func ProtectPrivateKeyDirectory(directory string) error {
	absolute, err := ProtectSecurityDirectory(directory)
	if err != nil {
		return err
	}
	keyPath := filepath.Join(absolute, "identity-key.pem")
	return ProtectSecurityFile(keyPath)
}

func ValidatePrivateKeySecurity(path string) error {
	return ValidateSecurityFile(path)
}

func ProtectSecurityDirectory(directory string) (string, error) {
	absolute, err := validatePrivateKeyDirectory(directory)
	if err != nil {
		return "", err
	}
	if err := applyCurrentUserDACL(absolute, true); err != nil {
		return "", fmt.Errorf("protect security directory: %w", err)
	}
	if err := validateExactCurrentUserDACL(absolute, objectInheritACE|containerInheritACE); err != nil {
		return "", fmt.Errorf("%w: security directory: %v", ErrInsecurePrivateKey, err)
	}
	return absolute, nil
}

func ValidateSecurityDirectory(directory string) (string, error) {
	absolute, err := validatePrivateKeyDirectory(directory)
	if err != nil {
		return "", err
	}
	if err := validateExactCurrentUserDACL(absolute, objectInheritACE|containerInheritACE); err != nil {
		return "", fmt.Errorf("%w: security directory: %v", ErrInsecurePrivateKey, err)
	}
	return absolute, nil
}

func ProtectSecurityFile(path string) error {
	absolute, err := validateRegularPathWithoutReparse(path)
	if err != nil {
		return err
	}
	if err := applyCurrentUserDACL(absolute, false); err != nil {
		return fmt.Errorf("protect security file: %w", err)
	}
	return ValidateSecurityFile(absolute)
}

func ValidateSecurityFile(path string) error {
	absolute, err := validateRegularPathWithoutReparse(path)
	if err != nil {
		return err
	}
	if err := validateExactCurrentUserDACL(filepath.Dir(absolute), objectInheritACE|containerInheritACE); err != nil {
		return fmt.Errorf("%w: credential directory: %v", ErrInsecurePrivateKey, err)
	}
	if err := validateExactCurrentUserDACL(absolute, 0); err != nil {
		return fmt.Errorf("%w: private key: %v", ErrInsecurePrivateKey, err)
	}
	return nil
}

func ProtectSecurityFileHandle(file *os.File) error {
	if file == nil {
		return errors.New("security file handle is required")
	}
	if err := protectSecurityHandle(file.Fd(), false); err != nil {
		return fmt.Errorf("protect security file handle: %w", err)
	}
	return ValidateSecurityFileHandle(file)
}

func ValidateSecurityFileHandle(file *os.File) error {
	if file == nil {
		return errors.New("security file handle is required")
	}
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if info.FileAttributes&fileAttributeReparsePoint != 0 || info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return fmt.Errorf("%w: security handle is not a regular file", ErrInsecurePrivateKey)
	}
	if info.NumberOfLinks != 1 {
		return fmt.Errorf("%w: security file has multiple hard links", ErrInsecurePrivateKey)
	}
	return validateExactCurrentUserDACLHandle(file.Fd(), 0)
}

func protectSecurityHandle(handle uintptr, directory bool) error {
	dacl, descriptor, err := currentUserDACL(directory)
	if err != nil {
		return err
	}
	defer syscall.LocalFree(syscall.Handle(descriptor))
	status, _, _ := procSetSecurityInfo.Call(
		handle,
		seFileObject,
		daclSecurityInformation|protectedDACLsecurityInformation,
		0,
		0,
		uintptr(unsafe.Pointer(dacl)),
		0,
	)
	runtime.KeepAlive(descriptor)
	if status != 0 {
		return syscall.Errno(status)
	}
	return nil
}

func currentUserDACL(directory bool) (*windowsACL, unsafe.Pointer, error) {
	userSID, err := currentUserSIDString()
	if err != nil {
		return nil, nil, err
	}
	flags := ""
	if directory {
		flags = "OICI"
	}
	aces := "(A;" + flags + ";FA;;;" + userSID + ")"
	if userSID != "S-1-5-18" {
		aces += "(A;" + flags + ";FA;;;SY)"
	}
	sddl, err := syscall.UTF16PtrFromString("D:P" + aces)
	if err != nil {
		return nil, nil, err
	}
	var descriptor unsafe.Pointer
	result, _, callErr := procConvertStringSecurityDescriptorToSecurityDescriptorW.Call(
		uintptr(unsafe.Pointer(sddl)), sddlRevision1, uintptr(unsafe.Pointer(&descriptor)), 0,
	)
	if result == 0 {
		return nil, nil, callErr
	}
	var dacl *windowsACL
	var present, defaulted int32
	result, _, callErr = procGetSecurityDescriptorDACL.Call(
		uintptr(descriptor), uintptr(unsafe.Pointer(&present)), uintptr(unsafe.Pointer(&dacl)), uintptr(unsafe.Pointer(&defaulted)),
	)
	if result == 0 || present == 0 || dacl == nil {
		_, _ = syscall.LocalFree(syscall.Handle(descriptor))
		if result == 0 {
			return nil, nil, callErr
		}
		return nil, nil, errors.New("generated security descriptor has no DACL")
	}
	return dacl, descriptor, nil
}

func validateExactCurrentUserDACLHandle(handle uintptr, expectedFlags byte) error {
	var dacl *windowsACL
	var descriptor unsafe.Pointer
	status, _, _ := procGetSecurityInfo.Call(
		handle,
		seFileObject,
		daclSecurityInformation,
		0,
		0,
		uintptr(unsafe.Pointer(&dacl)),
		0,
		uintptr(unsafe.Pointer(&descriptor)),
	)
	if status != 0 {
		return syscall.Errno(status)
	}
	defer syscall.LocalFree(syscall.Handle(descriptor))
	return validateCurrentUserDACL(dacl, descriptor, expectedFlags)
}

func validatePrivateKeyDirectory(directory string) (string, error) {
	if directory == "" {
		return "", errors.New("credential directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve credential directory: %w", err)
	}
	if err := rejectReparseComponents(absolute); err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect credential directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("credential path must be a directory")
	}
	return absolute, nil
}

func validateRegularPathWithoutReparse(path string) (string, error) {
	if path == "" {
		return "", errors.New("security file path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve security file path: %w", err)
	}
	if err := rejectReparseComponents(absolute); err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect security file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("security file must be a regular file")
	}
	return absolute, nil
}

func rejectReparseComponents(path string) error {
	volume := filepath.VolumeName(path)
	current := volume + string(os.PathSeparator)
	remainder := strings.TrimPrefix(path, current)
	for _, component := range strings.Split(remainder, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		pointer, err := syscall.UTF16PtrFromString(current)
		if err != nil {
			return err
		}
		attributes, _, callErr := procGetFileAttributesW.Call(uintptr(unsafe.Pointer(pointer)))
		if uint32(attributes) == invalidFileAttributes {
			return fmt.Errorf("inspect path attributes: %w", callErr)
		}
		if uint32(attributes)&fileAttributeReparsePoint != 0 {
			return fmt.Errorf("%w: reparse point in credential path", ErrInsecurePrivateKey)
		}
	}
	return nil
}

func currentUserSIDString() (string, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String()
}

func applyCurrentUserDACL(path string, directory bool) error {
	userSID, err := currentUserSIDString()
	if err != nil {
		return err
	}
	flags := ""
	if directory {
		flags = "OICI"
	}
	aces := "(A;" + flags + ";FA;;;" + userSID + ")"
	if userSID != "S-1-5-18" {
		aces += "(A;" + flags + ";FA;;;SY)"
	}
	sddl, err := syscall.UTF16PtrFromString("D:P" + aces)
	if err != nil {
		return err
	}
	var descriptor unsafe.Pointer
	result, _, callErr := procConvertStringSecurityDescriptorToSecurityDescriptorW.Call(
		uintptr(unsafe.Pointer(sddl)), sddlRevision1, uintptr(unsafe.Pointer(&descriptor)), 0,
	)
	if result == 0 {
		return callErr
	}
	defer syscall.LocalFree(syscall.Handle(descriptor))
	var dacl *windowsACL
	var present, defaulted int32
	result, _, callErr = procGetSecurityDescriptorDACL.Call(
		uintptr(descriptor), uintptr(unsafe.Pointer(&present)), uintptr(unsafe.Pointer(&dacl)), uintptr(unsafe.Pointer(&defaulted)),
	)
	if result == 0 || present == 0 || dacl == nil {
		if result == 0 {
			return callErr
		}
		return errors.New("generated security descriptor has no DACL")
	}
	pathPointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	status, _, _ := procSetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(pathPointer)), seFileObject,
		daclSecurityInformation|protectedDACLsecurityInformation,
		0, 0, uintptr(unsafe.Pointer(dacl)), 0,
	)
	runtime.KeepAlive(descriptor)
	if status != 0 {
		return syscall.Errno(status)
	}
	return nil
}

func validateExactCurrentUserDACL(path string, expectedFlags byte) error {
	pathPointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var dacl *windowsACL
	var descriptor unsafe.Pointer
	status, _, _ := procGetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(pathPointer)), seFileObject, daclSecurityInformation,
		0, 0, uintptr(unsafe.Pointer(&dacl)), 0, uintptr(unsafe.Pointer(&descriptor)),
	)
	if status != 0 {
		return syscall.Errno(status)
	}
	defer syscall.LocalFree(syscall.Handle(descriptor))
	return validateCurrentUserDACL(dacl, descriptor, expectedFlags)
}

func validateCurrentUserDACL(dacl *windowsACL, descriptor unsafe.Pointer, expectedFlags byte) error {
	if dacl == nil {
		return errors.New("DACL is missing")
	}
	var control uint16
	var revision uint32
	result, _, callErr := procGetSecurityDescriptorControl.Call(
		uintptr(descriptor), uintptr(unsafe.Pointer(&control)), uintptr(unsafe.Pointer(&revision)),
	)
	if result == 0 {
		return callErr
	}
	if control&seDACLProtected == 0 {
		return errors.New("DACL inheritance is enabled")
	}
	userSID, err := currentUserSIDString()
	if err != nil {
		return err
	}
	expected := map[string]bool{userSID: false}
	if userSID != "S-1-5-18" {
		expected["S-1-5-18"] = false
	}
	if int(dacl.ACECount) != len(expected) {
		return fmt.Errorf("DACL contains %d ACEs, want %d", dacl.ACECount, len(expected))
	}
	for index := uint32(0); index < uint32(dacl.ACECount); index++ {
		var acePointer unsafe.Pointer
		result, _, callErr = procGetACE.Call(uintptr(unsafe.Pointer(dacl)), uintptr(index), uintptr(unsafe.Pointer(&acePointer)))
		if result == 0 {
			return callErr
		}
		ace := (*windowsAccessAllowedACE)(acePointer)
		if ace.Header.Type != accessAllowedACEType || ace.Header.Flags != expectedFlags || ace.Mask != fileAllAccess {
			return errors.New("DACL contains a non-canonical access rule")
		}
		sid := (*syscall.SID)(unsafe.Pointer(&ace.SIDStart))
		sidString, err := sid.String()
		if err != nil {
			return err
		}
		seen, allowed := expected[sidString]
		if !allowed || seen {
			return errors.New("DACL grants access to an unexpected or duplicate principal")
		}
		expected[sidString] = true
	}
	for _, seen := range expected {
		if !seen {
			return errors.New("DACL is missing a required principal")
		}
	}
	runtime.KeepAlive(descriptor)
	return nil
}
