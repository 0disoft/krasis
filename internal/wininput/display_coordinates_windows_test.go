//go:build windows

package wininput

import (
	"testing"
	"unsafe"
)

func TestDisplayModeABI(t *testing.T) {
	var mode displayMode
	if unsafe.Sizeof(mode) != 220 || unsafe.Offsetof(mode.Size) != 68 || unsafe.Offsetof(mode.Width) != 172 || unsafe.Offsetof(mode.Height) != 176 {
		t.Fatal("DEVMODEW layout differs from Windows ABI")
	}
}
