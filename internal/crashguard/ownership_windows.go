//go:build windows

package crashguard

import (
	"errors"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Object existence is the fence. Mutex thread ownership/abandonment is never
// used: inherited references keep it alive after the resident process dies.
type portableOwnership struct {
	handle windows.Handle
	once   sync.Once
	err    error
}

func acquirePortableOwnership(name string) (*portableOwnership, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	dacl, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")(A;;GA;;;SY)")
	if err != nil {
		return nil, err
	}
	security := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: dacl, InheritHandle: 1}
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateMutex(&security, false, wide)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return nil, ErrExitTimeout
	}
	if err != nil {
		return nil, err
	}
	return &portableOwnership{handle: handle}, nil
}

func (ownership *portableOwnership) Close() error {
	ownership.once.Do(func() { ownership.err = windows.CloseHandle(ownership.handle) })
	return ownership.err
}

func (ownership *portableOwnership) argument() string {
	return strconv.FormatUint(uint64(ownership.handle), 10)
}

// adoptPortableOwnership consumes an inherited local bootstrap reference. It
// is lifetime ownership, never peer/service/desktop authorization. The caller
// must supply a handle from its closed inherited-handle allowlist.
func adoptPortableOwnership(value string) (*portableOwnership, error) {
	value64, err := strconv.ParseUint(value, 10, strconv.IntSize)
	if err != nil || value64 == 0 {
		return nil, errors.New("invalid portable ownership reference")
	}
	handle := windows.Handle(value64)
	if err := windows.SetHandleInformation(handle, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, err
	}
	return &portableOwnership{handle: handle}, nil
}

func pendingPortableOwnership(name string) (bool, error) {
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	handle, err := windows.OpenMutex(windows.SYNCHRONIZE, false, wide)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, windows.CloseHandle(handle)
}
