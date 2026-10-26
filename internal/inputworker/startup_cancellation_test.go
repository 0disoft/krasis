package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

func TestStartupCancellationHandoffIsIdempotent(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "canceled"}[canceled], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				peer, pipe := net.Pipe()
				defer peer.Close()
				channel := clientChannel(pipe, authorityService)
				defer channel.Close()
				var closed atomic.Int32
				channel.authority.closeHandles = func() error { closed.Add(1); return nil }
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				finish := startupCancellation(ctx, channel)
				defer finish()
				if canceled {
					cancel()
				}
				for range 2 {
					if err := finish(); (err != nil) != canceled || canceled && !errors.Is(err, context.Canceled) {
						t.Fatal("startup handoff lost cancellation", err)
					}
				}
				cancel()
				synctest.Wait()
				want := int32(0)
				if canceled {
					want = 1
				}
				if closed.Load() != want {
					t.Fatal("cancellation closed a transferred channel or failed to join cleanup", closed.Load())
				}
			})
		})
	}
}
