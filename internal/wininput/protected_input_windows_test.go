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
		{credentialForegroundBroker, ProtectedInputCredentialBroker},
		{credentialForegroundUnknown, ProtectedInputIdentityUnknown},
		{credentialForegroundState(255), ProtectedInputIdentityUnknown},
	} {
		if got := protectedCredentialReason(test.state); got != test.want {
			t.Fatalf("state=%d got=%d want=%d", test.state, got, test.want)
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
