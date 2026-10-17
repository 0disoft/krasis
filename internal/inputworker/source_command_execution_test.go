package inputworker

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

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
		if op == sourceActivate || op == sourceHeartbeat {
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
