//go:build windows

package netobserve

import (
	"syscall"
	"unsafe"
)

type systemPowerStatus struct {
	ACLineStatus, BatteryFlag, BatteryLifePercent, SystemStatusFlag byte
	BatteryLifeTime, BatteryFullLifeTime                            uint32
}

var getSystemPowerStatus = syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

func powerStatus() (Power, Saver) {
	var s systemPowerStatus
	ok, _, _ := getSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s)))
	return decodePower(s, ok != 0)
}
func decodePower(s systemPowerStatus, known bool) (Power, Saver) {
	if !known {
		return PowerUnknown, SaverUnknown
	}
	power, saver := PowerUnknown, SaverUnknown
	switch s.ACLineStatus {
	case 0:
		power = PowerBattery
	case 1:
		power = PowerAC
	}
	switch s.SystemStatusFlag {
	case 0:
		saver = SaverOff
	case 1:
		saver = SaverOn
	}
	return power, saver
}
