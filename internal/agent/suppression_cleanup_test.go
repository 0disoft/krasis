package agent

import (
	"errors"
	"sync/atomic"
	"testing"
)

type delayedCaptureCleanup struct {
	entered, release chan struct{}
	fail             bool
	active           atomic.Bool
}

func (c *delayedCaptureCleanup) Activate() error {
	c.active.Store(true)
	if c.fail {
		return errors.New("capture preparation failed")
	}
	return nil
}
func (c *delayedCaptureCleanup) Active() bool { return c.active.Load() }
func (c *delayedCaptureCleanup) Deactivate()  { close(c.entered); <-c.release; c.active.Store(false) }

func TestSuppressionStopsBeforeBlockedCaptureCleanup(t *testing.T) {
	for _, failedPreparation := range []bool{false, true} {
		source := &recordingActivation{order: new([]string)}
		capture := &delayedCaptureCleanup{entered: make(chan struct{}), release: make(chan struct{}), fail: failedPreparation}
		activation := &combinedSourceActivation{source: source, capture: capture}
		done := make(chan error, 1)
		go func() {
			err := activation.Activate()
			if !failedPreparation {
				activation.Deactivate()
			}
			done <- err
		}()
		<-capture.entered
		// Channel synchronization establishes the source state before cleanup blocks.
		if source.Active() {
			t.Error("capture cleanup delayed local suppression release")
		}
		close(capture.release)
		if err := <-done; (err != nil) != failedPreparation {
			t.Fatalf("preparation failure=%v error=%v", failedPreparation, err)
		}
		if activation.Active() {
			t.Fatal("input ownership remained active after cleanup")
		}
	}
}
