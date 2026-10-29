package inputworker

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestNativeTimingDoesNotWaitBehindCommandGate(t *testing.T) {
	client := &Client{sequence: 2}
	client.mu.Lock()
	defer client.mu.Unlock()
	var timing workerTiming
	called := false
	err := journalExecution(&isolatedInput{Client: client}, nil, &timing)(context.Background(), func() error { called = true; return nil })
	if err != nil || !called || timing.measured != 4 {
		t.Fatal("diagnostic gate changed input", err, called, timing)
	}
}

func TestNativeReplyTimingMeasuresSameCommandWithoutReplayingOrCaching(t *testing.T) {
	for _, mode := range []string{"single", "composite", "idle", "dropped", "revoked"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var native workerTiming
				checks, desktopChecks := 0, 0
				revoked := false
				execute := nativeInputExecutor(func(ctx context.Context, work func() error) error {
					desktopChecks++
					time.Sleep(8 * time.Millisecond)
					return work()
				}, func() error {
					checks++
					time.Sleep(12 * time.Millisecond)
					if revoked {
						return ErrAuthority
					}
					return nil
				}, func() bool { return true }, time.Now().Add(time.Minute), false, &native)
				transport := &entryTransport{handle: portableInputHandler(&recordingInput{}, execute, false, &native)}
				client, err := newPortableClient(context.Background(), transport)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				// Seed an earlier matching reply, then prove idle, rejection and
				// lost ACK cannot reuse it as this journal command's observation.
				if err := client.Move(context.Background(), 1, 0); err != nil {
					t.Fatal(err)
				}
				checks, desktopChecks = 0, 0
				transport.drop = mode == "dropped"
				revoked = mode == "revoked"
				var parent workerTiming
				journal := &isolatedInput{Client: client}
				err = journalExecution(journal, nil, &parent)(context.Background(), func() error {
					if mode == "idle" {
						return nil
					}
					if err := journal.Move(context.Background(), 1, 0); err != nil {
						return err
					}
					if mode == "composite" {
						return journal.Move(context.Background(), 1, 0)
					}
					return nil
				})
				if (err != nil) != (mode == "dropped" || mode == "revoked") {
					t.Fatal(mode, err)
				}
				wantChecks := 1
				if mode == "composite" {
					wantChecks = 2
				}
				if mode == "idle" {
					wantChecks = 0
				}
				if checks != wantChecks || len(transport.commands) != 1+wantChecks {
					t.Fatal("native query skipped or input replayed", mode, checks, len(transport.commands))
				}
				if mode == "single" {
					if parent.measured != 7 || parent.millis[0] != 12 || parent.millis[1] != 8 || parent.millis[2] != 20 || desktopChecks != 1 {
						t.Fatal("native stages lost or added to IPC", parent, desktopChecks)
					}
				} else if parent.measured != 4 || parent.buckets[0] != 0 || parent.buckets[1] != 0 {
					t.Fatal("stale/composite/failed reply published native timing", mode, parent)
				}
				if mode == "revoked" && desktopChecks != 0 {
					t.Fatal("revoked service entered desktop")
				}
			})
		})
	}
}
