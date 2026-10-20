//go:build windows

package netobserve

import (
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var capabilityRuntime = syscall.NewLazyDLL("combase.dll")
var capabilityInitialize = capabilityRuntime.NewProc("RoInitialize")
var capabilityUninitialize = capabilityRuntime.NewProc("RoUninitialize")
var capabilityFactory = capabilityRuntime.NewProc("RoGetActivationFactory")
var capabilityString = capabilityRuntime.NewProc("WindowsCreateString")
var capabilityDeleteString = capabilityRuntime.NewProc("WindowsDeleteString")

type capabilityObject struct{ vtable *[10]uintptr }

func (o *capabilityObject) release() { syscall.SyscallN(o.vtable[2], uintptr(unsafe.Pointer(o))) }
func hresultOK(hr uintptr) bool      { return int32(uint32(hr)) >= 0 }

// CheckAccess never requests permission. In particular slot 8 on the capability
// instance is RequestAccessAsync and is deliberately never invoked. ABI slots
// and IID match the Windows SDK IAppCapabilityStatics/IAppCapability headers.
func wifiLocationAllowed() bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, p := range []*syscall.LazyProc{capabilityInitialize, capabilityUninitialize, capabilityFactory, capabilityString, capabilityDeleteString} {
		if p.Find() != nil {
			return false
		}
	}
	hr, _, _ := capabilityInitialize.Call(1) // RO_INIT_MULTITHREADED; S_FALSE must also be balanced.
	if !hresultOK(hr) {
		return false
	}
	defer capabilityUninitialize.Call()
	makeString := func(value string) uintptr {
		utf16, err := syscall.UTF16FromString(value)
		if err != nil {
			return 0
		}
		var handle uintptr
		hr, _, _ := capabilityString.Call(uintptr(unsafe.Pointer(&utf16[0])), uintptr(len(utf16)-1), uintptr(unsafe.Pointer(&handle)))
		if !hresultOK(hr) {
			return 0
		}
		return handle
	}
	class := makeString("Windows.Security.Authorization.AppCapabilityAccess.AppCapability")
	if class == 0 {
		return false
	}
	defer capabilityDeleteString.Call(class)
	iid, err := windows.GUIDFromString("{7c353e2a-46ee-44e5-af3d-6ad3fc49bd22}")
	if err != nil {
		return false
	}
	var factory *capabilityObject
	hr, _, _ = capabilityFactory.Call(class, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&factory)))
	if !hresultOK(hr) || factory == nil {
		return false
	}
	defer factory.release()
	name := makeString("wiFiControl")
	if name == 0 {
		return false
	}
	defer capabilityDeleteString.Call(name)
	var capability *capabilityObject
	hr, _, _ = syscall.SyscallN(factory.vtable[8], uintptr(unsafe.Pointer(factory)), name, uintptr(unsafe.Pointer(&capability)))
	if !hresultOK(hr) || capability == nil {
		return false
	}
	defer capability.release()
	var status int32
	hr, _, _ = syscall.SyscallN(capability.vtable[9], uintptr(unsafe.Pointer(capability)), uintptr(unsafe.Pointer(&status)))
	return hresultOK(hr) && status == 4 // Allowed only, never UserPromptRequired.
}
