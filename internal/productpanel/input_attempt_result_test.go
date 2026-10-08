package productpanel

import (
	"context"
	"errors"
	"testing"
)

func TestHealthProbesAndRechecksCannotEraseLastFailedAttempt(t *testing.T) {
	for _, failure := range []error{ErrInputRefreshCancelled, ErrInputRefreshFailed} {
		t.Run(failure.Error(), func(t *testing.T) {
			state, calls := "UPDATE_REQUIRED", 0
			b := &inputRefresh{inspect: func() string { return state }, run: func(context.Context) error {
				calls++
				if calls == 1 {
					return failure
				}
				state = "READY"
				return nil
			}}
			if !errors.Is(b.automatic(context.Background()), failure) {
				t.Fatal("original failure missing")
			}
			want := "input_refresh_failed"
			if errors.Is(failure, ErrInputRefreshCancelled) {
				want = "input_refresh_cancelled"
			}
			state = "READY" // A later health observation is independent of this failed attempt.
			if b.probe(context.Background()) != "READY" || b.attemptResult() != want {
				t.Fatal("probe erased attempt", b.attemptResult())
			}
			b.recheck()
			b.mu.Lock()
			b.mu.Unlock()
			if b.attemptResult() != want || !errors.Is(b.automatic(context.Background()), failure) || calls != 1 {
				t.Fatal("recheck or startup claimed success/replayed consent")
			}
			state = "UPDATE_REQUIRED"
			if b.explicit(context.Background()) != nil || calls != 2 || b.attemptResult() != "ok" {
				t.Fatal("new explicit attempt did not replace old result")
			}
		})
	}
}
