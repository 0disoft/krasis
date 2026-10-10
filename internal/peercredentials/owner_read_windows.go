//go:build windows

package peercredentials

import (
	"errors"
	"os"
	"syscall"

	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/windows"
)

// OwnerReadScope is read-only validation for a service's administrator-selected
// profile owner. It grants no Windows access and never changes existing ACLs.
// Ordinary callers can select only themselves; SYSTEM must obtain the SID from
// protected configuration, never from a remote peer or an ordinary API request.
type OwnerReadScope struct{ sid string }

func ReadScopeForOwner(sid string) (*OwnerReadScope, error) {
	parsed, err := windows.StringToSid(sid)
	if err != nil || !parsed.IsValid() || parsed.String() != sid {
		return nil, ErrInsecurePrivateKey
	}
	current, err := currentUserSIDString()
	if err != nil || current != sid && current != "S-1-5-18" {
		return nil, ErrInsecurePrivateKey
	}
	return &OwnerReadScope{sid: sid}, nil
}

func (s *OwnerReadScope) OpenDirectory(path string) (*securefs.Directory, error) {
	if s == nil || s.sid == "" {
		return nil, ErrInsecurePrivateKey
	}
	dir, err := securefs.OpenReadDirectory(path)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateDirectory(dir.Handle()); err != nil {
		return nil, errors.Join(err, dir.Close())
	}
	return dir, nil
}

func (s *OwnerReadScope) ValidateDirectory(handle uintptr) error {
	if s == nil || s.sid == "" || handle == 0 {
		return ErrInsecurePrivateKey
	}
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(handle), &info); err != nil {
		return err
	}
	if info.FileAttributes&fileAttributeReparsePoint != 0 || info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return ErrInsecurePrivateKey
	}
	return validateExactOwnerDACLHandle(handle, objectInheritACE|containerInheritACE, s.sid)
}

func (s *OwnerReadScope) ValidateFile(file *os.File) error {
	if s == nil || s.sid == "" || file == nil {
		return ErrInsecurePrivateKey
	}
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if info.FileAttributes&(fileAttributeReparsePoint|syscall.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
		return ErrInsecurePrivateKey
	}
	return validateExactOwnerDACLHandle(file.Fd(), 0, s.sid)
}
