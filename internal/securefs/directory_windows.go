//go:build windows

package securefs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	deleteAccess             = 0x00010000
	standardRightsRead       = 0x00020000
	writeDAC                 = 0x00040000
	synchronizeAccess        = 0x00100000
	fileListDirectory        = 0x00000001
	fileReadData             = 0x00000001
	fileWriteData            = 0x00000002
	fileAppendData           = 0x00000004
	fileReadEA               = 0x00000008
	fileWriteEA              = 0x00000010
	fileReadAttributes       = 0x00000080
	fileWriteAttributes      = 0x00000100
	fileGenericRead          = standardRightsRead | fileReadData | fileReadAttributes | fileReadEA | synchronizeAccess
	fileGenericWrite         = standardRightsRead | fileWriteData | fileWriteAttributes | fileWriteEA | fileAppendData | synchronizeAccess
	fileShareRead            = 0x00000001
	fileShareWrite           = 0x00000002
	fileShareDelete          = 0x00000004
	fileOpen                 = 0x00000001
	fileCreate               = 0x00000002
	fileDirectoryFile        = 0x00000001
	fileWriteThrough         = 0x00000002
	fileSynchronousNonAlert  = 0x00000020
	fileNonDirectoryFile     = 0x00000040
	fileOpenForBackupIntent  = 0x00004000
	fileOpenReparsePoint     = 0x00200000
	objCaseInsensitive       = 0x00000040
	objDontReparse           = 0x00001000
	fileRenameInformation    = 10
	fileDispositionInfo      = 13
	fileDispositionInfoEx    = 64
	fileRenameInformationEx  = 65
	fileRenameReplace        = 0x00000001
	fileRenamePOSIX          = 0x00000002
	fileDispositionDelete    = 0x00000001
	fileDispositionPOSIX     = 0x00000002
	fileDispositionIgnoreRO  = 0x00000010
	statusNameCollision      = 0xC0000035
	statusReparseEncountered = 0xC000050B
)

type ntUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

type objectAttributes struct {
	Length             uint32
	RootDirectory      syscall.Handle
	ObjectName         *ntUnicodeString
	Attributes         uint32
	SecurityDescriptor unsafe.Pointer
	SecurityQoS        unsafe.Pointer
}

type ioStatusBlock struct {
	Status      uint32
	Information uintptr
}

type renameInformation struct {
	ReplaceIfExists bool
	RootDirectory   syscall.Handle
	FileNameLength  uint32
	FileName        [syscall.MAX_PATH]uint16
}

type renameInformationEx struct {
	Flags          uint32
	RootDirectory  syscall.Handle
	FileNameLength uint32
	FileName       [syscall.MAX_PATH]uint16
}

type dispositionInformation struct {
	DeleteFile bool
}

type dispositionInformationEx struct {
	Flags uint32
}

var (
	ntdll                     = syscall.NewLazyDLL("ntdll.dll")
	procNtCreateFile          = ntdll.NewProc("NtCreateFile")
	procNtSetInformationFile  = ntdll.NewProc("NtSetInformationFile")
	procRtlNtStatusToDosError = ntdll.NewProc("RtlNtStatusToDosError")
)

type Directory struct {
	path   string
	handle syscall.Handle
	once   sync.Once
}

func OpenDirectory(path string) (*Directory, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if filepath.VolumeName(absolute) == "" || strings.HasPrefix(absolute, `\\`) {
		return nil, ErrUnsafePath
	}
	handle, err := ntCreate(
		syscall.InvalidHandle,
		`\??\`+absolute,
		fileListDirectory|fileReadAttributes|synchronizeAccess,
		fileOpen,
		fileDirectoryFile|fileOpenForBackupIntent|fileOpenReparsePoint|fileSynchronousNonAlert,
	)
	if err != nil {
		return nil, fmt.Errorf("open secure directory: %w", err)
	}
	return &Directory{path: absolute, handle: handle}, nil
}

func (directory *Directory) Close() error {
	if directory == nil || directory.handle == syscall.InvalidHandle || directory.handle == 0 {
		return nil
	}
	var err error
	directory.once.Do(func() { err = syscall.CloseHandle(directory.handle) })
	return err
}

func (directory *Directory) Handle() uintptr {
	if directory == nil {
		return 0
	}
	return uintptr(directory.handle)
}

func (directory *Directory) Open(name string) (*os.File, error) {
	return directory.openExisting(name, fileGenericRead)
}

func (directory *Directory) OpenReadWrite(name string) (*os.File, error) {
	return directory.openExisting(name, fileGenericRead|fileGenericWrite|deleteAccess)
}

func (directory *Directory) CreateExclusive(name string, mode os.FileMode) (*os.File, error) {
	if err := directory.ready(name); err != nil {
		return nil, err
	}
	handle, err := ntCreate(
		directory.handle,
		name,
		fileGenericRead|fileGenericWrite|deleteAccess|writeDAC,
		fileCreate,
		fileNonDirectoryFile|fileOpenReparsePoint|fileSynchronousNonAlert|fileWriteThrough,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), filepath.Join(directory.path, name))
	if file == nil {
		_ = syscall.CloseHandle(handle)
		return nil, errors.New("wrap secure file handle")
	}
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func (directory *Directory) CreateTemporary(prefix string, mode os.FileMode) (*os.File, string, error) {
	for attempt := 0; attempt < 16; attempt++ {
		name, err := temporaryName(prefix)
		if err != nil {
			return nil, "", err
		}
		file, err := directory.CreateExclusive(name, mode)
		if err == nil {
			return file, name, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", errors.New("secure temporary name collision limit reached")
}

func (directory *Directory) openExisting(name string, access uint32) (*os.File, error) {
	if err := directory.ready(name); err != nil {
		return nil, err
	}
	handle, err := ntCreate(
		directory.handle,
		name,
		access|fileReadAttributes|synchronizeAccess,
		fileOpen,
		fileNonDirectoryFile|fileOpenReparsePoint|fileSynchronousNonAlert,
	)
	if err != nil {
		return nil, err
	}
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &info); err != nil {
		_ = syscall.CloseHandle(handle)
		return nil, err
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = syscall.CloseHandle(handle)
		return nil, ErrUnsafePath
	}
	if info.NumberOfLinks != 1 {
		_ = syscall.CloseHandle(handle)
		return nil, ErrHardLink
	}
	file := os.NewFile(uintptr(handle), filepath.Join(directory.path, name))
	if file == nil {
		_ = syscall.CloseHandle(handle)
		return nil, errors.New("wrap secure file handle")
	}
	return file, nil
}

func (directory *Directory) Replace(source, destination string) error {
	return directory.rename(source, destination, true)
}

func (directory *Directory) PromoteNoReplace(source, destination string) error {
	return directory.rename(source, destination, false)
}

func (directory *Directory) Remove(name string) error {
	if err := directory.ready(name); err != nil {
		return err
	}
	handle, err := ntCreate(
		directory.handle,
		name,
		deleteAccess|fileReadAttributes|synchronizeAccess,
		fileOpen,
		fileNonDirectoryFile|fileOpenReparsePoint|fileSynchronousNonAlert,
	)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(handle)
	info := dispositionInformationEx{Flags: fileDispositionDelete | fileDispositionPOSIX | fileDispositionIgnoreRO}
	if status := ntSetInformation(handle, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), fileDispositionInfoEx); status == 0 {
		return nil
	}
	fallback := dispositionInformation{DeleteFile: true}
	status := ntSetInformation(handle, unsafe.Pointer(&fallback), uint32(unsafe.Sizeof(fallback)), fileDispositionInfo)
	return ntStatusError(status)
}

func (directory *Directory) rename(source, destination string, replace bool) error {
	if err := directory.ready(source); err != nil {
		return err
	}
	if err := validateLeaf(destination); err != nil {
		return err
	}
	handle, err := ntCreate(
		directory.handle,
		source,
		deleteAccess|fileReadAttributes|synchronizeAccess,
		fileOpen,
		fileNonDirectoryFile|fileOpenReparsePoint|fileSynchronousNonAlert,
	)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(handle)
	var fileInfo syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &fileInfo); err != nil {
		return err
	}
	if fileInfo.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrUnsafePath
	}
	if fileInfo.NumberOfLinks != 1 {
		return ErrHardLink
	}
	name, err := syscall.UTF16FromString(destination)
	if err != nil || len(name) > syscall.MAX_PATH {
		return ErrUnsafePath
	}
	flags := uint32(fileRenamePOSIX)
	if replace {
		flags |= fileRenameReplace
	}
	ex := renameInformationEx{Flags: flags, RootDirectory: directory.handle}
	copy(ex.FileName[:], name)
	ex.FileNameLength = uint32((len(name) - 1) * 2)
	status := ntSetInformation(handle, unsafe.Pointer(&ex), uint32(unsafe.Sizeof(ex)), fileRenameInformationEx)
	if status == 0 {
		return nil
	}
	basic := renameInformation{ReplaceIfExists: replace, RootDirectory: directory.handle}
	copy(basic.FileName[:], name)
	basic.FileNameLength = ex.FileNameLength
	status = ntSetInformation(handle, unsafe.Pointer(&basic), uint32(unsafe.Sizeof(basic)), fileRenameInformation)
	return ntStatusError(status)
}

func (directory *Directory) ready(name string) error {
	if directory == nil || directory.handle == syscall.InvalidHandle || directory.handle == 0 {
		return errors.New("secure directory is closed")
	}
	return validateLeaf(name)
}

func ntCreate(root syscall.Handle, name string, access, disposition, options uint32) (syscall.Handle, error) {
	buffer, err := syscall.UTF16FromString(name)
	if err != nil || len(buffer)*2 > int(^uint16(0)) {
		return syscall.InvalidHandle, ErrUnsafePath
	}
	unicodeName := ntUnicodeString{
		Length:        uint16((len(buffer) - 1) * 2),
		MaximumLength: uint16(len(buffer) * 2),
		Buffer:        &buffer[0],
	}
	attributes := objectAttributes{
		Length:     uint32(unsafe.Sizeof(objectAttributes{})),
		ObjectName: &unicodeName,
		Attributes: objCaseInsensitive | objDontReparse,
	}
	if root != syscall.InvalidHandle {
		attributes.RootDirectory = root
	}
	var handle syscall.Handle
	var iosb ioStatusBlock
	status, _, _ := procNtCreateFile.Call(
		uintptr(unsafe.Pointer(&handle)),
		uintptr(access),
		uintptr(unsafe.Pointer(&attributes)),
		uintptr(unsafe.Pointer(&iosb)),
		0,
		uintptr(syscall.FILE_ATTRIBUTE_NORMAL),
		uintptr(fileShareRead|fileShareWrite|fileShareDelete),
		uintptr(disposition),
		uintptr(options),
		0,
		0,
	)
	runtime.KeepAlive(buffer)
	if status != 0 {
		return syscall.InvalidHandle, ntStatusError(uint32(status))
	}
	return handle, nil
}

func ntSetInformation(handle syscall.Handle, information unsafe.Pointer, size, class uint32) uint32 {
	var iosb ioStatusBlock
	status, _, _ := procNtSetInformationFile.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&iosb)),
		uintptr(information),
		uintptr(size),
		uintptr(class),
	)
	return uint32(status)
}

func ntStatusError(status uint32) error {
	if status == 0 {
		return nil
	}
	switch status {
	case statusNameCollision:
		return syscall.ERROR_FILE_EXISTS
	case statusReparseEncountered:
		return ErrUnsafePath
	}
	converted, _, _ := procRtlNtStatusToDosError.Call(uintptr(status))
	if converted == 0 {
		return syscall.EINVAL
	}
	return syscall.Errno(converted)
}
