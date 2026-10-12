//go:build windows

package wininput

import "testing"

func TestProtectedCredentialReasonKeepsUnknownFailClosed(t *testing.T) {
	for _, test := range []struct {
		state credentialForegroundState
		want  ProtectedInputReason
	}{
		{credentialForegroundMissing, ProtectedInputClear},
		{credentialForegroundOther, ProtectedInputClear},
		{credentialForegroundLockScreen, ProtectedInputClear},
		{credentialForegroundBroker, ProtectedInputCredentialBroker},
		{credentialForegroundUnknown, ProtectedInputIdentityUnknown},
		{credentialForegroundState(255), ProtectedInputIdentityUnknown},
	} {
		if got := protectedCredentialReason(test.state); got != test.want {
			t.Fatalf("state=%d got=%d want=%d", test.state, got, test.want)
		}
	}
}

func TestLockScreenImageDoesNotMatchOtherExecutables(t *testing.T) {
	for _, path := range []string{`C:\Windows\SystemApps\LockApp.exe`, `C:\Windows\SystemApps\LOCKAPP.EXE`} {
		if !isLockScreenImage(path) {
			t.Fatal("lock-screen image not recognized")
		}
	}
	for _, path := range []string{`C:\LockApp.exe.bak`, `C:\LockApp.exe\chrome.exe`} {
		if isLockScreenImage(path) {
			t.Fatal("unrelated foreground selected protected presentation")
		}
	}
}

func TestCredentialBrokerImageIdentification(t *testing.T) {
	for _, test := range []struct {
		path string
		want bool
	}{
		{`C:\Windows\System32\CredentialUIBroker.exe`, true},
		{`c:\windows\system32\CREDENTIALUIBROKER.EXE`, true},
		{`C:\Program Files\Google\Chrome\Application\chrome.exe`, false},
		{`C:\Windows\System32\CredentialUIBroker.exe.bak`, false},
	} {
		if got := isCredentialBrokerImage(test.path); got != test.want {
			t.Errorf("isCredentialBrokerImage(%q) = %t, want %t", test.path, got, test.want)
		}
	}
}
