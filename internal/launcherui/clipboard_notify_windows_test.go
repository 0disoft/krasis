//go:build windows

package launcherui

import "testing"

func TestClipboardWakeCoalescesWithoutBlockingWindowProcedure(t *testing.T) {
	wake := make(chan struct{}, 1)
	WakeClipboard(nil)
	for i := 0; i < 100; i++ {
		WakeClipboard(wake)
	}
	if len(wake) != 1 {
		t.Fatal("notifications not coalesced")
	}
	<-wake
	WakeClipboard(wake)
	if len(wake) != 1 {
		t.Fatal("next notification lost")
	}
}
