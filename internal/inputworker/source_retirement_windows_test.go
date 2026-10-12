//go:build windows

package inputworker

import (
	"context"
	"testing"
)

func TestSourceStopConsumesAlreadyConfirmedRetirementWithoutAnotherCommand(t *testing.T) {
	live := &sourceClient{done: make(chan struct{})}
	live.retired.Store(true)
	close(live.done)
	if _, err := live.call(context.Background(), sourceStop); err != nil {
		t.Fatal("attempted another command after exact native retirement", err)
	}
}
