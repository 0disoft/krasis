//go:build windows

package inputworker

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Reproduce a delayed reader wakeup without sleeping past the idle budget:
// complete the kernel read, then expire/cancel its caller before observation.
func TestNativePipeCompletedReadSurvivesLateDeadlineOrCancellation(t *testing.T) {
	for _, mode := range []string{"deadline", "cancellation"} {
		t.Run(mode, func(t *testing.T) {
			server, client := nativePair(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := server.start(); err != nil {
				t.Fatal(err)
			}
			defer server.active.Done()
			var data [1]byte
			written := make(chan error, 1)
			n, err := server.overlapped(ctx, true, data[:], func(o *windows.Overlapped) error {
				var immediate uint32
				if err := windows.ReadFile(server.handle, data[:], &immediate, o); err != windows.ERROR_IO_PENDING {
					t.Fatalf("expected a pending kernel read: %v", err)
				}
				go func() { _, err := client.Write([]byte{7}); written <- err }()
				state, err := windows.WaitForSingleObject(o.HEvent, 1000)
				if err != nil || state != windows.WAIT_OBJECT_0 {
					t.Fatalf("read did not complete: %v %d", err, state)
				}
				if mode == "deadline" {
					_ = server.SetReadDeadline(time.Now().Add(-time.Second))
				} else {
					cancel()
				}
				return windows.ERROR_IO_PENDING
			})
			if writeErr := <-written; writeErr != nil {
				t.Fatal(writeErr)
			}
			if err != nil || n != 1 || data[0] != 7 {
				t.Fatalf("completed input was discarded: n=%d err=%v", n, err)
			}
		})
	}
}
