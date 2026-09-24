//go:build windows

package wininput

import "testing"

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
