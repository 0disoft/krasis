//go:build windows

package inputworker

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestInputReturnPreparesOnceWithoutMovementHintAndTransfersUnboundChild(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lease := Lease{ExpiresAt: time.Now().Add(time.Minute)}
		child := &preparedConsole{}
		calls := 0
		pending := prepareInputReentry(context.Background(), lease, nil, func(context.Context) (*preparedConsole, error) {
			calls++
			return child, nil
		})
		if pending == nil {
			t.Fatal("clean return requires another movement hint")
		}
		got, err := pending.join()
		if got != child || err != nil || calls != 1 || child.warmState.Load() != 0 {
			t.Fatal("return lost or bound the unclaimed preparation", err)
		}
		if again, err := pending.join(); again != nil || err != nil || calls != 1 {
			t.Fatal("return preparation transferred twice", err)
		}
		if child.close() != nil {
			t.Fatal("child cleanup failed")
		}
	})
}

func TestInputReturnDoesNotPrepareAfterFailureExpiryOrOtherModes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		name  string
		ctx   context.Context
		lease Lease
		err   error
	}{
		{"nil context", nil, Lease{ExpiresAt: time.Now().Add(time.Minute)}, nil},
		{"cancelled", ctx, Lease{ExpiresAt: time.Now().Add(time.Minute)}, nil},
		{"expired", context.Background(), Lease{ExpiresAt: time.Now().Add(-time.Second)}, nil},
		{"capture", context.Background(), Lease{Capture: true, ExpiresAt: time.Now().Add(time.Minute)}, nil},
		{"presentation", context.Background(), Lease{Presentation: true, ExpiresAt: time.Now().Add(time.Minute)}, nil},
		{"authority failure", context.Background(), Lease{ExpiresAt: time.Now().Add(time.Minute)}, ErrAuthority},
		{"recovery unknown", context.Background(), Lease{ExpiresAt: time.Now().Add(time.Minute)}, errors.Join(context.Canceled, ErrRecovery)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if pending := prepareInputReentry(test.ctx, test.lease, test.err, func(context.Context) (*preparedConsole, error) {
				t.Fatal("ineligible return launched a child")
				return nil, nil
			}); pending != nil {
				t.Fatal("ineligible return queued preparation")
			}
		})
	}
}
