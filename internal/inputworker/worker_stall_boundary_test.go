package inputworker

import (
	"context"
	"net"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type stalledRecoveryBoundaryInput struct {
	*recordingInput
	entered, unblock chan struct{}
}

func (input *stalledRecoveryBoundaryInput) Move(context.Context, int32, int32) error {
	close(input.entered)
	<-input.unblock // Simulate a native call that does not observe cancellation.
	return nil
}

// This diagnoses the current single-worker boundary, not independent recovery
// acceptance. A caller deadline or process-exit receipt cannot prove key-up.
// No native input or process termination occurs in this controlled test.
func TestHeldWorkerRecoveryWaitsForUncooperativeInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &stalledRecoveryBoundaryInput{recordingInput: &recordingInput{}, entered: make(chan struct{}), unblock: make(chan struct{})}
		var resume sync.Once
		defer resume.Do(func() { close(input.unblock) })
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		server, client := net.Pipe()
		defer client.Close()
		lease := testLease()
		done := make(chan finish, 1)
		go func() {
			report, err := serve(ctx, server, input, lease, testExecute, testExecute, time.Now)
			done <- finish{report, err}
		}()
		send(t, client, request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}})
		data, err := encodeRequest(request{op: opMove, sequence: 2, binding: lease.Binding, x: 1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Write(data[:]); err != nil {
			t.Fatal(err)
		}
		<-input.entered
		cancel()
		time.Sleep(2 * RecoveryTimeout) // Virtual time; the native stand-in remains blocked.
		select {
		case result := <-done:
			t.Fatal("blocked input fabricated worker retirement", result)
		default:
		}
		input.mu.Lock()
		released := input.released
		input.mu.Unlock()
		if released != 0 {
			t.Fatal("recovery raced an input operation that could still apply")
		}
		resume.Do(func() { close(input.unblock) })
		result := <-done
		if result.err == nil || result.report.Recovery.Released != 1 || result.report.Recovery.Remaining != 0 {
			t.Fatal("native return did not preserve independent-context cleanup", result)
		}
	})
}
