package inputworker

import "github.com/0disoft/krasis/internal/inputdesktop"

// Versions 8..14 mirror legacy versions 1..7 exactly. Only two otherwise-unused
// certainty bits carry a diagnostic target. No suffix, extra I/O, or request is
// added. Legacy peers reject the new version; installed image matching remains
// the compatibility boundary. Unknown enum values drop metadata, not an ACK.
func legacyResponseVersion(version byte) byte {
	if version >= 8 && version <= 14 {
		return version - 7
	}
	return version
}

func attachDesktopDiagnostic(data []byte, desktop inputdesktop.DesktopClass) {
	if desktop != inputdesktop.DesktopDefault && desktop != inputdesktop.DesktopWinlogon {
		return
	}
	data[4] += 7
	data[7] |= byte(desktop) << 6
}

func detachDesktopDiagnostic(data *[responseSize]byte) inputdesktop.DesktopClass {
	if data[4] < 8 || data[4] > 14 {
		return inputdesktop.DesktopUnknown
	}
	desktop := inputdesktop.DesktopClass(data[7] >> 6)
	data[4], data[7] = legacyResponseVersion(data[4]), data[7]&0x3f
	if desktop != inputdesktop.DesktopDefault && desktop != inputdesktop.DesktopWinlogon {
		return inputdesktop.DesktopUnknown
	}
	return desktop
}

const desktopObservationLimit = 32

// Called only after the response's op and sequence match on the authenticated
// worker pipe. At most 32 transitions plus one truncation event per lease.
func (client *Client) observeDesktop(desktop inputdesktop.DesktopClass) {
	if client.timing.emit == nil || desktop == client.lastDesktop || desktop != inputdesktop.DesktopDefault && desktop != inputdesktop.DesktopWinlogon {
		return
	}
	client.lastDesktop = desktop
	if client.desktopObservations > desktopObservationLimit {
		return
	}
	if client.desktopObservations == desktopObservationLimit {
		client.desktopObservations++
		emitTiming(client.timing.emit, "installed_input_desktop_limit")
		return
	}
	client.desktopObservations++
	code := "installed_input_desktop_default"
	if desktop == inputdesktop.DesktopWinlogon {
		code = "installed_input_desktop_winlogon"
	}
	emitTiming(client.timing.emit, code)
}
