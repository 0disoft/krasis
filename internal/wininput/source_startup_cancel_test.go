package wininput

import (
	"context"
	"errors"
	"testing"
)

func TestSourceRetiredBeforeWorkerStartsIsNormalCancellation(t *testing.T) {
	session, err := NewSourceSession(1, &recordingSourceSink{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session.Retire()
	ready := false
	if err := session.RunWithReady(ctx, func() { ready = true }); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup: %v", err)
	}
	if ready || session.Active() {
		t.Fatal("retired worker became ready")
	}
	if err := session.Run(context.Background()); err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("retired worker allowed restart: %v", err)
	}
}
