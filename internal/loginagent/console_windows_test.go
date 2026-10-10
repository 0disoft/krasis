//go:build windows

package loginagent

import "testing"

func TestReadCurrentConsoleOnly(t *testing.T) {
	console, err := ReadConsole()
	if err != nil {
		t.Fatal(err)
	}
	// This does not change session state or inspect the user name. The attended
	// workstation test proves only the native read path, not pre-login support.
	if console.State == Unknown || console.State > UserSession {
		t.Fatal("invalid native console state")
	}
}
