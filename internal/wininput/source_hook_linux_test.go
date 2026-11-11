package wininput

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/x11desktop"
)

func TestLinuxSourceCanceledClosedOwnerReleasesGrabsWithoutFault(t *testing.T) {
	if os.Getenv("KRASIS_X11_TEST") != "1" {
		t.Skip("requires isolated Xvfb; acquires keyboard and pointer grabs")
	}
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint("active=", active), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			session, err := NewSourceSession(1, &recordingSourceSink{})
			if err != nil {
				t.Fatal(err)
			}
			hook := NewSourceHook()
			ready := false
			err = hook.Run(ctx, session, func() {
				ready = true
				if active {
					if err := hook.ActivateProfile(ctx); err != nil {
						t.Fatal(err)
					}
				}
				// Reproduce do(ctx)'s cancellation closure deterministically.
				_ = hook.desktop.Close()
				cancel()
			})
			if !ready {
				t.Fatalf("source setup failed before cancellation fixture: %v", err)
			}
			if err != context.Canceled {
				t.Fatalf("normal cancellation gained a cleanup failure: %v", err)
			}
			select {
			case err := <-session.faults:
				t.Fatalf("normal cancellation aborted the source: %v", err)
			default:
			}
			if hook.Active() {
				t.Fatal("capture remains active")
			}
			probeCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
			defer stop()
			probe, err := x11desktop.Open(probeCtx)
			if err != nil {
				t.Fatal(err)
			}
			defer probe.Close()
			if err := probe.Grab(probeCtx); err != nil {
				t.Fatalf("closed source retained a grab: %v", err)
			}
			if err := probe.Ungrab(probeCtx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
