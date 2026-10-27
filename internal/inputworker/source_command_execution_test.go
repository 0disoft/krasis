package inputworker

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/peer"
)

func TestSourceExecutedCommandReplyRetainsProtocolShape(t *testing.T) {
	for op := sourceStart; op <= sourceCursorRestore; op++ {
		if op == sourceStop {
			continue
		} // Stop sends a retirement frame instead.
		for _, rejected := range []bool{false, true} {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				timing, err := executeSourceCommand(context.Background(), op, func(_ context.Context, work func() error) error {
					time.Sleep(20 * time.Millisecond)
					return work()
				}, func() error {
					calls++
					time.Sleep(20 * time.Millisecond)
					if rejected {
						return ErrAuthority
					}
					return nil
				})
				reply := sourcePacket{kind: sourceReply, op: op, sequence: 1, reply: 1, binding: Binding{1}, timing: timing}
				if err != nil {
					reply.status = 1
				} else if op == sourceAdmit {
					reply.decision = peer.BoundaryAdmissionAllowed
				}
				wire, encodeErr := encodeSourcePacket(reply)
				got, readErr := readSourcePacket(bytes.NewReader(wire[:]))
				if encodeErr != nil || readErr != nil || got != reply || calls != 1 || (err != nil) != rejected {
					t.Fatal("execution produced an invalid/mutated reply", op, rejected, timing, encodeErr, readErr)
				}
			})
		}
	}
}

func TestSourceActivationDefersFullAuthorizationUntilDesktopExecution(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		full, presence, applied := 0, 0, 0
		revoked := false
		check := func() error {
			full++
			time.Sleep(80 * time.Millisecond) // The duplicate RPC removed from admission.
			if revoked {
				return ErrAuthority
			}
			return nil
		}
		present := func() error { presence++; return nil }
		wire, _ := encodeSourcePacket(sourcePacket{kind: sourceCommand, op: sourceActivate, sequence: 1, binding: Binding{1}})
		started := time.Now()
		p, err := readSourceCommandAuthorized(bytes.NewReader(wire[:]), context.Background(), Binding{1}, 0, func(op byte) error {
			return sourceCommandReadAuthorization(op, check, present)
		})
		if err != nil || full != 0 || presence != 1 || time.Since(started) != 0 {
			t.Fatal("activation admission repeated full RPC", p, err, full, presence)
		}
		// Revocation while queued must be observed at the real execution boundary.
		revoked = true
		timing, err := executeSourceCommand(context.Background(), p.op, func(ctx context.Context, work func() error) error {
			if err := check(); err != nil {
				return err
			}
			return work()
		}, func() error { applied++; return nil })
		if err != ErrAuthority || applied != 0 || full != 1 || timing != ([3]byte{0, 3, 0}) {
			t.Fatal("queued activation escaped fresh authorization or invented apply timing", err, full, applied, timing)
		}
	})
}

func TestSourceActivationKeepsScopeAndNativeFailureFromOneAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failure := errors.New("synthetic uncertain native outcome")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		calls := 0
		timing, err := executeSourceCommand(ctx, sourceActivate, func(actual context.Context, work func() error) error {
			if actual != ctx {
				t.Fatal("execution replaced request deadline")
			}
			time.Sleep(80 * time.Millisecond)
			return work()
		}, func() error { calls++; time.Sleep(120 * time.Millisecond); return failure })
		if err != failure || calls != 1 || timing != ([3]byte{0, 3, 4}) {
			t.Fatal("operation retried, failure hidden or scope included native span", calls, err, timing)
		}
	})
}

func TestSourceCommandRetainsReadChecksAndCleanupDesktopPolicy(t *testing.T) {
	for op := sourceStart; op <= sourceCursorRestore; op++ {
		full, presence := 0, 0
		err := sourceCommandReadAuthorization(op, func() error { full++; return ErrAuthority }, func() error { presence++; return ErrProtocol })
		if op == sourceActivate || op == sourceHeartbeat || op == sourceAdmit {
			if presence != 1 || full != 0 || err != ErrProtocol {
				t.Fatal("missing pinned presence", op, presence, full, err)
			}
		} else if full != 1 || presence != 0 || err != ErrAuthority {
			t.Fatal("other read-side authority changed", op, presence, full, err)
		}
		usedDesktop, applied := false, false
		err = nil
		_, err = executeSourceCommand(context.Background(), op, func(context.Context, func() error) error { usedDesktop = true; return ErrAuthority }, func() error { applied = true; return nil })
		cleanup := op == sourceDeactivate || op == sourceHeartbeat || op == sourceCursorRestore
		if cleanup && (usedDesktop || !applied || err != nil) || !cleanup && (!usedDesktop || applied || err != ErrAuthority) {
			t.Fatal("command escaped mandatory desktop or cleanup acquired one", op, usedDesktop, applied, err)
		}
	}
}
