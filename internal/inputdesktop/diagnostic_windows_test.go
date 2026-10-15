//go:build windows

package inputdesktop

import "testing"

func TestNativeDesktopDiagnosticRequiresExactVerifiedHandle(t *testing.T) {
	for _, target := range []desktopTarget{defaultTarget, winlogonTarget} {
		b := &desktopBinding{target: target, owned: 1}
		if b.diagnosticDesktop() != DesktopUnknown {
			t.Fatal("unverified handle inferred")
		}
		b.verifiedHandle = 2
		if b.diagnosticDesktop() != DesktopUnknown {
			t.Fatal("another verified handle inferred")
		}
		b.verifiedHandle = 1
		want := DesktopDefault
		if target == winlogonTarget {
			want = DesktopWinlogon
		}
		if b.diagnosticDesktop() != want {
			t.Fatal("fixed target lost")
		}
		b.owned = 0
		if b.diagnosticDesktop() != DesktopUnknown {
			t.Fatal("closed native target inferred")
		}
	}
}
