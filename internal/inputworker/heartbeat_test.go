package inputworker

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestHeartbeatUsesRecentAcknowledgementWithoutExtraRoundtrip(t *testing.T) {
	conn, done := startWorker(t, context.Background(), &recordingInput{}, testLease())
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var events []string
	client.SetTimingObserver(func(code string) { events = append(events, code) })
	if err := client.Move(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := client.Heartbeat(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.sequence != 2 {
		t.Error("heartbeat inserted another roundtrip immediately after acknowledged input")
	}
	_ = client.Close()
	for _, event := range events {
		if strings.Contains(event, "heartbeat_timing") {
			t.Error("skipped heartbeat fabricated timing measurements")
		}
	}
	if result := await(t, done); result.err != nil || result.report.Completed != 1 {
		t.Fatal("heartbeat changed input completion or recovery")
	}
}

func TestHeartbeatDoesNotQueueBehindBusyInput(t *testing.T) {
	conn, done := startWorker(t, context.Background(), &recordingInput{}, testLease())
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	returned := make(chan error, 1)
	go func() { returned <- client.Heartbeat(context.Background()) }()
	select {
	case err := <-returned:
		if err != nil {
			t.Error("busy input was treated as failed heartbeat")
		}
	case <-time.After(time.Second):
		t.Error("background heartbeat queued behind the input mutex")
	}
	client.mu.Unlock()
	_ = client.Close()
	_ = await(t, done)
}

func TestIdleHeartbeatRetainsSequenceAndAuthorityChecks(t *testing.T) {
	conn, done := startWorker(t, context.Background(), &recordingInput{}, testLease())
	channel := clientChannel(conn, authorityService)
	client, err := NewClient(context.Background(), channel, testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var events []string
	client.SetTimingObserver(func(code string) { events = append(events, code) })
	if err := client.Heartbeat(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.Move(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	client.lastAcknowledged = time.Now().Add(-time.Second)
	client.mu.Unlock()
	if err := client.Heartbeat(context.Background()); err != nil || client.sequence != 4 {
		t.Fatal("idle heartbeat omitted or consumed the wrong sequence")
	}
	channel.authority.checkInput = func() error { return ErrAuthority }
	if err := client.Move(context.Background(), 1, 1); err == nil || client.sequence != 4 {
		t.Fatal("recent acknowledgement bypassed input authority")
	}
	_ = client.Close()
	if result := await(t, done); result.err != nil || result.report.Completed != 1 {
		t.Fatal("maintenance altered input completion")
	}
	inputStages, heartbeatStages := 0, 0
	for _, event := range events {
		if strings.HasPrefix(event, "installed_input_heartbeat_timing_") {
			heartbeatStages++
		} else {
			inputStages++
		}
	}
	// This legacy test worker does not report worker timings, so each class
	// contributes just its four measured client stages.
	if inputStages != 4 || heartbeatStages != 4 {
		t.Fatal("input and maintenance maxima mixed or repeated on close")
	}
}

func TestRecentAcknowledgementCannotExtendLeaseOrHideFailure(t *testing.T) {
	conn, done := startWorker(t, context.Background(), &recordingInput{}, testLease())
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Move(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	client.lease.ExpiresAt = time.Now().Add(-time.Second)
	client.mu.Unlock()
	if err := client.Heartbeat(context.Background()); err == nil {
		t.Fatal("recent acknowledgement extended expired lease")
	}
	if err := client.Heartbeat(context.Background()); err == nil {
		t.Fatal("failed channel hidden by recent acknowledgement")
	}
	_ = client.Close()
	_ = await(t, done)
}

func TestInputHeartbeatUsesPresenceWithoutWeakeningInputAuthority(t *testing.T) {
	for _, mode := range []string{"present", "exited", "fallback"} {
		t.Run(mode, func(t *testing.T) {
			lease := testLease()
			conn, done := startWorker(t, context.Background(), &recordingInput{}, lease)
			channel := clientChannel(conn, authorityService)
			client, err := NewClient(context.Background(), channel, lease)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			fullChecks, presenceChecks := 0, 0
			channel.authority.checkInput = func() error { fullChecks++; return ErrAuthority }
			if mode != "fallback" {
				channel.authority.checkPresent = func() error {
					presenceChecks++
					if mode == "exited" {
						return ErrAuthority
					}
					return nil
				}
			}
			err = client.Heartbeat(context.Background())
			if mode == "present" {
				if err != nil || fullChecks != 0 || presenceChecks != 1 || client.sequence != 2 {
					t.Fatal("heartbeat repeated full authorization instead of its pinned owner check", err, fullChecks, presenceChecks)
				}
				if err := client.Move(context.Background(), 1, 0); err == nil || fullChecks != 1 || client.sequence != 2 {
					t.Fatal("heartbeat presence bypassed full input authorization", err)
				}
			} else if err == nil || client.sequence != 1 || mode == "fallback" && fullChecks != 1 || mode == "exited" && presenceChecks != 1 {
				t.Fatal("heartbeat accepted a missing owner or skipped its fallback authority", err, fullChecks, presenceChecks)
			}
			_ = client.Close()
			if result := await(t, done); result.err != nil || result.report.Completed != 0 {
				t.Fatal("maintenance failure injected input or bypassed worker cleanup", result.err)
			}
		})
	}
}
