//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
)

func TestIdleCaptureCancellationDoesNotRetireSender(t *testing.T) {
	for _, tc := range []struct {
		name        string
		paused      bool
		cause       error
		wantRetired bool
	}{
		{"verified idle replacement", true, ErrSourceCaptureRefresh, false},
		{"ordinary shutdown", true, context.Canceled, true},
		{"unfenced replacement", false, ErrSourceCaptureRefresh, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, _ := NewSourceSession(4, &recordingSourceSink{})
			if tc.paused && !session.PauseCapture() {
				t.Fatal("pause failed")
			}
			ctx, cancel := context.WithCancelCause(context.Background())
			cancel(tc.cause)
			current := &sourceWheelCaptureRun{session: session}
			current.recordContextStop(ctx)
			if session.terminal.Load() != tc.wantRetired || !errors.Is(current.err(), context.Canceled) {
				t.Fatal("wrong sender cancellation policy", session.terminal.Load(), current.err())
			}
			current.fail(errors.New("native cleanup failed"))
			if !session.terminal.Load() {
				t.Fatal("cleanup failure did not retire sender")
			}
		})
	}
}

func TestPointerSnapshotFallbackRejectsJoinedCleanupFailure(t *testing.T) {
	snapshot := sourceStage("pointer-snapshot", errors.New("unavailable"))
	if !SourcePointerSnapshotFailureOnly(errors.Join(snapshot)) {
		t.Fatal("snapshot-only failure lost")
	}
	if SourcePointerSnapshotFailureOnly(errors.Join(snapshot, errors.New("native close failed"))) {
		t.Fatal("cleanup failure hidden as a snapshot race")
	}
}
