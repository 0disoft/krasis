//go:build windows

package wininput

import "testing"

func TestProductForegroundNeverAttachesForeignInputQueue(t *testing.T) {
	if err := sourceWheelCaptureAttachThreads(11, 22, true); err == nil {
		t.Fatal("product thread attachment unexpectedly enabled")
	}
}

func TestRetiredSourceRejectsLateActivation(t *testing.T) {
	session, _ := NewSourceSession(1, &recordingSourceSink{})
	session.started.Store(true)
	session.physicalMu.Lock()
	done := make(chan error, 1)
	go func() { done <- session.Activate() }()
	session.Retire()
	session.physicalMu.Unlock()
	if err := <-done; err == nil || session.Active() {
		t.Fatal("retired source reactivated")
	}
}
