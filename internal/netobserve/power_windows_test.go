//go:build windows

package netobserve

import (
	"testing"
	"unsafe"
)

func TestPowerStatusABIAndUnknownResults(t *testing.T) {
	if unsafe.Sizeof(systemPowerStatus{}) != 12 {
		t.Fatal("SYSTEM_POWER_STATUS ABI mismatch")
	}
	for _, tc := range []struct {
		native systemPowerStatus
		known  bool
		power  Power
		saver  Saver
	}{
		{systemPowerStatus{ACLineStatus: 1, SystemStatusFlag: 0}, true, PowerAC, SaverOff},
		{systemPowerStatus{ACLineStatus: 0, SystemStatusFlag: 1}, true, PowerBattery, SaverOn},
		{systemPowerStatus{ACLineStatus: 255, SystemStatusFlag: 255}, true, PowerUnknown, SaverUnknown},
		{systemPowerStatus{}, false, PowerUnknown, SaverUnknown},
	} {
		power, saver := decodePower(tc.native, tc.known)
		if power != tc.power || saver != tc.saver {
			t.Fatal("native power state misclassified", power, saver)
		}
	}
}
