package wininput

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/cleanupresult"
	"github.com/0disoft/krasis/internal/x11desktop"
)

func TestLinuxSourceRejectsOverlappingSetupBeforeOpeningX11(t *testing.T) {
	hook := NewSourceHook()
	hook.running = true // the first Run owns initialization, before desktop is set
	session, err := NewSourceSession(1, &recordingSourceSink{})
	if err != nil {
		t.Fatal(err)
	}
	err = hook.Run(context.Background(), session, nil)
	if err == nil || err.Error() != "source hook already running" || cleanupresult.Confirmed(err) {
		t.Fatal("overlapping setup inherited another owner's cleanup proof", err)
	}
	if !hook.running {
		t.Fatal("overlapping Run released original owner's fence")
	}
}

func TestLinuxSourceFailedClosedOwnerConfirmsNativeRetirement(t *testing.T) {
	if os.Getenv("KRASIS_X11_TEST") != "1" {
		t.Skip("requires isolated Xvfb; acquires native grabs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := NewSourceSession(1, &recordingSourceSink{})
	if err != nil {
		t.Fatal(err)
	}
	hook := NewSourceHook()
	err = hook.Run(ctx, session, func() {
		if err := hook.ActivateProfile(ctx); err != nil {
			t.Fatal(err)
		}
		_ = hook.desktop.Close() // native failure, NOT parent cancellation
	})
	if !cleanupresult.Confirmed(err) || !errors.Is(err, net.ErrClosed) || ctx.Err() != nil || hook.Active() {
		t.Fatal("native failure or retirement proof lost", err)
	}
	probe, err := x11desktop.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if err := probe.Grab(ctx); err != nil {
		t.Fatal("failed owner retained grab", err)
	}
	if err := probe.Ungrab(ctx); err != nil {
		t.Fatal(err)
	}
}

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
