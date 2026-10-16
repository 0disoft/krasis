package inputworker

import (
	"context"
	"io"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/application"
)

type entryBudgetInput struct {
	cursorInput
	snapshotDelay, placementDelay       time.Duration
	snapshotDeadline, placementDeadline time.Time
	placements                          int
}

func (p *entryBudgetInput) CursorPosition(ctx context.Context) (int32, int32, error) {
	p.snapshotDeadline, _ = ctx.Deadline()
	time.Sleep(p.snapshotDelay) // Models a native query that cannot be interrupted.
	return p.x, p.y, nil
}

func (p *entryBudgetInput) PlaceCursor(ctx context.Context, x, y int32) (application.CursorPlacement, error) {
	p.placementDeadline, _ = ctx.Deadline()
	if ctx.Err() != nil {
		return application.CursorPlacement{}, notSent(ctx.Err())
	}
	p.placements++
	p.x, p.y = x, y
	time.Sleep(p.placementDelay) // A submitted native write may finish after expiry.
	return application.CursorPlacement{TargetX: x, TargetY: y, X: x, Y: y}, nil
}

func TestCursorEntrySharesSnapshotPlacementAndOriginalACKDeadline(t *testing.T) {
	for _, test := range []struct {
		name                string
		snapshot, placement time.Duration
		writes              int
		success             bool
	}{
		{"within_original_budget", 1950 * time.Millisecond, 20 * time.Millisecond, 1, true},
		{"placement_crosses_deadline", 1950 * time.Millisecond, 100 * time.Millisecond, 1, false},
		{"snapshot_exhausts_budget", 2010 * time.Millisecond, 0, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				server, pipe := net.Pipe()
				input := &entryBudgetInput{cursorInput: cursorInput{x: 10, y: 20}, snapshotDelay: test.snapshot, placementDelay: test.placement}
				done := make(chan finish, 1)
				go func() {
					r, err := serve(context.Background(), server, input, testLease(), testExecute, testExecute, time.Now)
					done <- finish{r, err}
				}()
				client, err := NewClient(context.Background(), clientChannel(pipe, authorityService), testLease())
				if err != nil {
					t.Fatal(err)
				}
				started := time.Now()
				entry, err := client.EnterCursor(context.Background(), 5, 6)
				elapsed := time.Since(started)
				if test.success {
					if err != nil || entry.BeforeX != 10 || entry.BeforeY != 20 || entry.X != 5 || entry.Y != 6 {
						t.Fatal("entry within budget failed", err)
					}
				} else {
					if c, ok := application.InjectionCertaintyOf(err); !ok || c != application.InjectionOutcomeUnknown || entry != (application.CursorEntry{}) {
						t.Fatal("expired ACK fabricated success or a rollback snapshot", err)
					}
					if elapsed > IdleTimeout {
						t.Fatal("entry renewed its client budget", elapsed)
					}
				}
				if client.sequence != 2 {
					t.Fatal("entry was replayed")
				}
				_ = client.Close()
				_ = await(t, done)
				if input.snapshotDeadline != input.placementDeadline || input.snapshotDeadline != started.Add(IdleTimeout) || input.placements != test.writes {
					t.Fatal("placement got a new deadline or wrote after expiry", input)
				}
			})
		})
	}
}

type lostEntryACK struct{ net.Conn }

func (c lostEntryACK) Write([]byte) (int, error) { _ = c.Conn.Close(); return 0, io.ErrClosedPipe }

func TestAppliedCursorEntryWithLostACKNeverReplaysOrReturnsSnapshot(t *testing.T) {
	server, pipe := net.Pipe()
	input := &entryBudgetInput{cursorInput: cursorInput{x: 10, y: 20}}
	done := make(chan finish, 1)
	go func() {
		r, err := serve(context.Background(), lostEntryACK{server}, input, testLease(), testExecute, testExecute, time.Now)
		done <- finish{r, err}
	}()
	client, err := NewClient(context.Background(), clientChannel(pipe, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	entry, err := client.EnterCursor(context.Background(), 5, 6)
	result := await(t, done)
	if input.placements != 1 || input.x != 5 || input.y != 6 || result.report.Completed != 1 {
		t.Fatal("fixture did not apply entry before losing ACK")
	}
	if c, ok := application.InjectionCertaintyOf(err); !ok || c != application.InjectionOutcomeUnknown || entry != (application.CursorEntry{}) {
		t.Fatal("lost ACK exposed rollback state", err)
	}
	if _, err := client.EnterCursor(context.Background(), 5, 6); err == nil || client.sequence != 2 || input.placements != 1 {
		t.Fatal("uncertain applied entry replayed")
	}
}

func TestCursorEntryTransitionDrainKeepsMatchingHardStopAndDeadlinePolicy(t *testing.T) {
	for _, op := range []operation{opCursorEntry, opCursorEntryProbe} {
		for _, mode := range []string{"matched", "foreign", "hard_stop", "deadline", "lease_expiry", "manual_cancel", "incomplete_write"} {
			t.Run(map[operation]string{opCursorEntry: "entry/", opCursorEntryProbe: "probe/"}[op]+mode, func(t *testing.T) {
				server, pipe := net.Pipe()
				defer server.Close()
				_ = server.SetDeadline(time.Now().Add(time.Second))
				stream := &ackWaitConn{Conn: pipe, waiting: make(chan struct{})}
				lease := testLease()
				if mode == "lease_expiry" {
					lease.ExpiresAt = time.Now().Add(100 * time.Millisecond)
				}
				client, err := NewClient(context.Background(), clientChannel(stream, authorityService), lease)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				hardStop, stop := context.WithCancel(context.Background())
				defer stop()
				intent := &drainLifetime{Context: hardStop, checked: make(chan struct{})}
				attempt, cancel := context.WithCancelCause(context.Background())
				defer cancel(context.Canceled)
				ctx := WithDesktopTransitionDrain(attempt, intent)
				if mode == "deadline" {
					var finish context.CancelFunc
					ctx, finish = context.WithTimeout(ctx, 100*time.Millisecond)
					defer finish()
				}
				callbacks := 0
				done := make(chan error, 1)
				go func() {
					done <- client.performResponse(ctx, request{op: op, x: 5, y: 6}, func(response) { callbacks++ })
				}()
				if mode == "incomplete_write" {
					var data [1]byte
					if _, err := server.Read(data[:]); err != nil {
						t.Fatal(err)
					}
					cancel(&agentlifecycle.TransitionError{Event: agentlifecycle.EventDesktopSwitch})
					if err := awaitClient(t, done); err == nil || callbacks != 0 || client.sequence != 2 {
						t.Fatal("incomplete entry waited or published success", err)
					}
					return
				}
				command, err := readRequest(server)
				if err != nil {
					t.Fatal(err)
				}
				<-stream.waiting
				cause := error(&agentlifecycle.TransitionError{Event: agentlifecycle.EventDesktopSwitch})
				if mode == "manual_cancel" {
					cause = context.Canceled
				}
				cancel(cause)
				if mode != "manual_cancel" {
					<-intent.checked
				}
				if mode == "hard_stop" {
					stop()
				}
				if mode == "matched" || mode == "foreign" {
					ack := response{op: command.op, sequence: command.sequence, beforeX: 10, beforeY: 20, x: 5, y: 6, targetX: 5, targetY: 6}
					if mode == "foreign" {
						ack.sequence++
					}
					if err := writeResponse(server, ack); err != nil {
						t.Fatal(err)
					}
				}
				err = awaitClient(t, done)
				if mode == "matched" {
					if err != nil || callbacks != 1 {
						t.Fatal("matching submitted entry ACK lost", err)
					}
				} else if c, ok := application.InjectionCertaintyOf(err); !ok || c != application.InjectionOutcomeUnknown || callbacks != 0 {
					t.Fatal("failed entry published snapshot or changed certainty", err)
				}
				if client.sequence != 2 {
					t.Fatal("entry retransmitted after transition")
				}
			})
		}
	}
}
