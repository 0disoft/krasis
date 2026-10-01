//go:build windows

package inputworker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Installation is constructed only from administrator-owned fixed files. A
// zero value cannot authorize service startup; JSON supplied by an agent or
// update request is never an installation source.
type Installation struct {
	mu                         sync.Mutex
	closed                     bool
	agentDigest, serviceDigest [sha256.Size]byte
	files                      []*os.File
}

type installationRecord struct {
	Schema  string `json:"schemaVersion"`
	Agent   string `json:"agentSHA256"`
	Service string `json:"serviceSHA256"`
}

func LoadInstallation() (*Installation, error) {
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return nil, ErrAuthority
	}
	return loadInstallationDirectory(filepath.Join(root, "Krasis"), filepath.Join(root, "Krasis", "Input"))
}

// Private directory selection is shared with the elevated installer for its
// three fixed staging locations. It is never selected by service/agent IPC.
func loadInstallationDirectory(root, directory string) (*Installation, error) {
	installation := &Installation{}
	accepted := false
	defer func() {
		if !accepted {
			_ = installation.Close()
		}
	}()
	for _, path := range []string{root, directory} {
		file, err := openProtected(path, true)
		if err != nil {
			return nil, err
		}
		installation.files = append(installation.files, file)
	}
	metadata, err := openProtected(filepath.Join(directory, "INPUT-INSTALL.json"), false)
	if err != nil {
		return nil, err
	}
	installation.files = append(installation.files, metadata)
	data, err := io.ReadAll(io.LimitReader(metadata, 4097))
	if err != nil || len(data) > 4096 {
		return nil, ErrAuthority
	}
	agent, service, err := decodeInstallation(data)
	if err != nil {
		return nil, ErrAuthority
	}
	installation.agentDigest, installation.serviceDigest = agent, service
	for _, image := range []struct {
		name   string
		digest [sha256.Size]byte
	}{{"krasis-agent.exe", agent}, {"krasis-input.exe", service}} {
		file, err := lockImage(filepath.Join(directory, image.name), image.digest)
		if err != nil {
			return nil, ErrAuthority
		}
		installation.files = append(installation.files, file)
		if protectedHandle(windows.Handle(file.Fd())) != nil {
			return nil, ErrAuthority
		}
	}
	accepted = true
	return installation, nil
}

func decodeInstallation(data []byte) ([sha256.Size]byte, [sha256.Size]byte, error) {
	var agent, service [sha256.Size]byte
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record installationRecord
	if decoder.Decode(&record) != nil || record.Schema != "krasis.input.install/v1" || decoder.Decode(new(any)) != io.EOF {
		return agent, service, ErrAuthority
	}
	for _, field := range []struct {
		text   string
		output *[sha256.Size]byte
	}{{record.Agent, &agent}, {record.Service, &service}} {
		value, err := hex.DecodeString(field.text)
		if err != nil || len(value) != sha256.Size {
			return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrAuthority
		}
		copy(field.output[:], value)
		if *field.output == ([sha256.Size]byte{}) {
			return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrAuthority
		}
	}
	return agent, service, nil
}

func openProtected(path string, directory bool) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, ErrAuthority
	}
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, installNativeError("file_open", ErrAuthority, err)
	}
	file := os.NewFile(uintptr(handle), "protected-input-installation")
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		file.Close()
		return nil, installNativeError("file_attributes", ErrAuthority, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || (!directory && info.NumberOfLinks != 1) {
		_ = file.Close()
		return nil, ErrAuthority
	}
	if err := protectedHandle(handle); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func protectedHandle(handle windows.Handle) error {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return installNativeError("file_security", ErrAuthority, err)
	}
	return protectedDescriptor(descriptor)
}

func administratorSID(sid *windows.SID) bool {
	return sid != nil && sid.IsValid() && (sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid))
}

func protectedDescriptor(descriptor *windows.SECURITY_DESCRIPTOR) error {
	if descriptor == nil {
		return ErrAuthority
	}
	owner, _, err := descriptor.Owner()
	if err != nil || !administratorSID(owner) {
		return ErrAuthority
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount > 256 {
		return ErrAuthority
	}
	// Win32 FILE_DELETE_CHILD (0x40) is not exported by x/sys v0.46.
	const fileDeleteChild = 0x40
	const writes = uint32(windows.GENERIC_ALL | windows.GENERIC_WRITE | windows.MAXIMUM_ALLOWED | windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES | fileDeleteChild)
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, i, &ace) != nil || ace == nil || ace.Header.AceSize < 16 {
			return ErrAuthority
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrAuthority
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.IsValid() || sid.Len() > int(ace.Header.AceSize)-8 {
			return ErrAuthority
		}
		if uint32(ace.Mask)&writes != 0 && !administratorSID(sid) {
			return ErrAuthority
		}
	}
	return nil
}

func (installation *Installation) digests() ([sha256.Size]byte, [sha256.Size]byte, error) {
	if installation == nil {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrAuthority
	}
	installation.mu.Lock()
	defer installation.mu.Unlock()
	if installation.closed || len(installation.files) != 5 || installation.agentDigest == ([sha256.Size]byte{}) || installation.serviceDigest == ([sha256.Size]byte{}) {
		return [sha256.Size]byte{}, [sha256.Size]byte{}, ErrAuthority
	}
	return installation.agentDigest, installation.serviceDigest, nil
}

func (installation *Installation) Close() error {
	if installation == nil {
		return nil
	}
	installation.mu.Lock()
	defer installation.mu.Unlock()
	if installation.closed {
		return nil
	}
	installation.closed = true
	var result error
	for _, file := range installation.files {
		if file.Close() != nil {
			result = ErrAuthority
		}
	}
	return result
}
