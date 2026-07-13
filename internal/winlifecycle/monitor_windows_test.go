//go:build windows

package winlifecycle

import (
	"testing"
	"time"
)

func TestMonitorStartsAndClosesHiddenWindow(t *testing.T) {
	monitor, err := StartMonitor()
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- monitor.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Windows lifecycle monitor did not close")
	}
}
