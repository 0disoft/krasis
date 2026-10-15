//go:build windows

package inputdesktop

func (b *desktopBinding) diagnosticDesktop() DesktopClass {
	if b == nil || b.owned == 0 || b.verifiedHandle != b.owned {
		return DesktopUnknown
	}
	switch b.target {
	case defaultTarget:
		return DesktopDefault
	case winlogonTarget:
		return DesktopWinlogon
	}
	return DesktopUnknown
}
