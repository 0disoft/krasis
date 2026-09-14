//go:build windows

package wininput

import (
	"syscall"
	"unsafe"
)

// DEVMODEW's display union is opaque: diagnostics only consumes pixel dimensions.
type displayMode struct {
	Device                                        [32]uint16
	Spec, Driver, Size, Extra                     uint16
	Fields                                        uint32
	Display                                       [4]uint32
	Color, Duplex, YResolution, TTOption, Collate int16
	Form                                          [32]uint16
	LogPixels                                     uint16
	Bits, Width, Height                           uint32
	Remaining                                     [10]uint32
}

// DisplayCoordinates reads physical dimensions and process awareness without
// changing DPI context. Zero dimensions / -1 awareness mean unavailable.
func DisplayCoordinates() (uint32, uint32, int32) {
	mode := displayMode{Size: uint16(unsafe.Sizeof(displayMode{}))}
	result, _, _ := user32.NewProc("EnumDisplaySettingsW").Call(0, uintptr(0xffffffff), uintptr(unsafe.Pointer(&mode)))
	if result == 0 {
		mode.Width, mode.Height = 0, 0
	}
	awareness := int32(-1)
	proc := syscall.NewLazyDLL("shcore.dll").NewProc("GetProcessDpiAwareness")
	if proc.Find() == nil {
		var value int32
		hr, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&value)))
		if hr == 0 && value >= 0 && value <= 2 {
			awareness = value
		}
	}
	return mode.Width, mode.Height, awareness
}
