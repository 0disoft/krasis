package inputworker

import (
	"context"
	"testing"
	"testing/synctest"
)

// The completion slot removes a dispatcher/reader rendezvous, not the reader's
// wait for command completion. A pipelined request cannot overtake that wait.
func TestSourceCompletionSlotPreservesSerialReadAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		requests, completed := make(chan sourcePacket, 1), make(chan struct{}, 1)
		reads := 0
		done := make(chan error, 1)
		go func() {
			done <- pumpSourceCommandsOrdered(ctx, func() (sourcePacket, error) {
				reads++
				return sourcePacket{op: sourceActivate, sequence: uint64(reads)}, nil
			}, requests, cancel, completed, nil)
		}()
		if (<-requests).sequence != 1 {
			t.Fatal("first request lost")
		}
		synctest.Wait()
		if reads != 1 {
			t.Fatal("reader overtook unfinished command")
		}
		// Publishing completion must not read or dispatch another request here;
		// only the ordered reader may admit it after observing this receipt.
		select {
		case completed <- struct{}{}:
		default:
			t.Fatal("dispatcher waits for reader scheduling")
		}
		synctest.Wait()
		if reads != 2 || (<-requests).sequence != 2 {
			t.Fatal("next command reordered")
		}
		cancel()
		if err := <-done; err != context.Canceled {
			t.Fatal("cancelled completion wait leaked", err)
		}
	})
}
