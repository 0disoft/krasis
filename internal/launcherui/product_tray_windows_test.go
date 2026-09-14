//go:build windows

package launcherui

import (
	"strings"
	"testing"
	"unsafe"
)

func TestNotifyIconLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("x64 product ABI")
	}
	var d notifyIconData
	if unsafe.Sizeof(d) != 976 || unsafe.Offsetof(d.Icon) != 32 || unsafe.Offsetof(d.Tip) != 40 || unsafe.Offsetof(d.Version) != 816 {
		t.Fatal("NOTIFYICONDATAW x64 layout mismatch")
	}
}

func TestTrayLabelsDoNotExposeUntrustedState(t *testing.T) {
	for _, state := range []string{"LOCAL", "CONNECTED", "CONNECTING", "RECONNECTING", "DISCONNECTING", "DEGRADED", "FAULTED", "SAFE_LOCAL", "", "private-marker"} {
		label := trayLabel(state)
		if !strings.HasPrefix(label, "Krasis · ") || strings.Contains(label, "private-marker") {
			t.Fatal("invalid tray label")
		}
	}
	if trayLabel("LOCAL") == trayLabel("") || trayLabel("CONNECTED") == trayLabel("FAULTED") {
		t.Fatal("distinct states collapsed")
	}
}
