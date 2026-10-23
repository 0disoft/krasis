package runtimestall

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

func TestShutdownPreservesPendingFailureSnapshot(t *testing.T) {
	for _, stop := range []string{"cancel", "close", "expiry"} {
		for _, failSave := range []bool{false, true} {
			t.Run(stop+map[bool]string{false: "/saved", true: "/failed"}[failSave], func(t *testing.T) {
				// Both stop and failure are ready when the sample finishes. Every
				// select ordering must preserve exactly one terminal save attempt.
				for range 16 {
					synctest.Test(t, func(t *testing.T) {
						ctx, cancel := context.WithCancel(context.Background())
						defer cancel()
						f := &fakeSource{}
						entered, release := make(chan struct{}), make(chan struct{})
						var events []string
						writes := 0
						r := start(ctx, f, func(traceSource) error {
							writes++
							if writes == 1 {
								close(entered)
								<-release
							} else if failSave {
								return errors.New("terminal snapshot failure")
							}
							return nil
						}, func(code string) { events = append(events, code) }, time.Minute)
						r.RequestSample()
						<-entered
						r.Request()
						closed := make(chan struct{})
						switch stop {
						case "cancel":
							cancel()
						case "close":
							go func() { r.Close(); close(closed) }()
							synctest.Wait()
						case "expiry":
							time.Sleep(time.Minute)
						}
						close(release)
						<-r.done
						if stop == "close" {
							<-closed
						}
						r.Close()
						r.Request()
						r.RequestSample()
						want := "runtime_trace_saved"
						if failSave {
							want = "runtime_trace_save_failed"
						}
						if writes != 2 || !f.stopped.Load() || !slices.Equal(events, []string{"runtime_trace_enabled", "runtime_trace_sample_saved", want}) {
							t.Fatalf("writes=%d stopped=%t events=%v", writes, f.stopped.Load(), events)
						}
					})
				}
			})
		}
	}
}

func TestShutdownWithoutFailureDoesNotWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeSource{}
		r := start(context.Background(), f, func(traceSource) error {
			t.Error("shutdown without failure wrote a snapshot")
			return nil
		}, func(string) {}, time.Minute)
		r.Close()
		if !f.stopped.Load() {
			t.Fatal("source was not stopped")
		}
	})
}

func TestCloseRemainsBoundedDuringTerminalSave(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeSource{}
		entered, release := make(chan struct{}), make(chan struct{})
		writes := 0
		r := start(context.Background(), f, func(traceSource) error {
			writes++
			close(entered)
			<-release
			return nil
		}, func(string) {}, time.Minute)
		r.Request()
		<-entered
		started := time.Now()
		r.Close()
		elapsed := time.Since(started)
		if elapsed != 2*time.Second || f.stopped.Load() {
			t.Errorf("close took %v; stopped during save=%t", elapsed, f.stopped.Load())
		}
		close(release)
		<-r.done
		if writes != 1 || !f.stopped.Load() {
			t.Fatalf("writes=%d stopped=%t", writes, f.stopped.Load())
		}
	})
}
