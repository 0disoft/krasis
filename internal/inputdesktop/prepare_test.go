package inputdesktop

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInteractivePreparationSettlesBeforeDispatchWithoutReplayingInput(t *testing.T) {
	opens, authorizations, calls := 0, 0, 0
	desktop := &switchingDesktop{}
	b := &interactiveBinding{authorize: func() error { authorizations++; return nil }, open: func() (binding, error) {
		opens++
		if opens < 3 {
			return nil, desktopPreparing(ErrUnavailable)
		}
		return desktop, nil
	}}
	scope := &Scope{root: context.Background(), desktop: b, owner: 1, thread: func() uintptr { return 1 }}
	err := scope.Do(context.Background(), func() error { calls++; return nil })
	if err != nil || calls != 1 || opens != 3 || authorizations < 3 {
		t.Fatal("preparation did not reauthorize or dispatched more than once", err, calls, opens, authorizations)
	}
	failure := errors.New("injection already began")
	if err := scope.Do(context.Background(), func() error { calls++; return failure }); err != failure || calls != 2 || opens != 3 {
		t.Fatal("input failure replayed or desktop reopened", err, calls, opens)
	}
}

func TestInteractivePreparationPreservesDeadlineCancellationAndRevocation(t *testing.T) {
	for _, tc := range []struct {
		name           string
		cancel, revoke bool
	}{
		{"caller deadline", false, false}, {"caller cancellation", true, false}, {"revocation", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
			defer cancel()
			checks, opens := 0, 0
			b := &interactiveBinding{authorize: func() error {
				checks++
				if tc.revoke && checks > 1 {
					return ErrUnavailable
				}
				return nil
			}, open: func() (binding, error) {
				opens++
				if tc.cancel {
					cancel()
				}
				return nil, desktopPreparing(ErrUnavailable)
			}}
			err := b.activeContext(ctx, context.Background())
			if err == nil || opens > 2 || (tc.cancel && !errors.Is(err, context.Canceled)) ||
				(!tc.cancel && !tc.revoke && !errors.Is(err, context.DeadlineExceeded)) || (tc.revoke && opens != 1) {
				t.Fatal("preparation renewed budget or ignored stop/revocation", err, opens, checks)
			}
		})
	}
}

func TestInteractivePreparationIsBoundedAndCleanupFailureCannotRetry(t *testing.T) {
	opens := 0
	b := &interactiveBinding{authorize: func() error { return nil }, open: func() (binding, error) {
		opens++
		return nil, desktopPreparing(ErrUnavailable)
	}}
	if err := b.activeContext(context.Background(), context.Background()); !errors.Is(err, context.DeadlineExceeded) || opens > 12 {
		t.Fatal("unbounded desktop preparation", err, opens)
	}
	opens = 0
	b = &interactiveBinding{authorize: func() error { return nil }, open: func() (binding, error) {
		opens++
		return nil, desktopPreparing(errors.Join(ErrUnavailable, ErrCleanup))
	}}
	if err := b.activeContext(context.Background(), context.Background()); !errors.Is(err, ErrCleanup) || opens != 1 || !b.fenced {
		t.Fatal("cleanup failure retried or unfenced", err, opens)
	}
}

func TestInteractivePreparationStopsOnScopeCancellation(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	opens, calls := 0, 0
	b := &interactiveBinding{authorize: func() error { return nil }, open: func() (binding, error) {
		opens++
		cancel()
		return nil, desktopPreparing(ErrUnavailable)
	}}
	scope := &Scope{root: root, desktop: b, owner: 1, thread: func() uintptr { return 1 }}
	err := scope.Do(context.Background(), func() error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 0 || opens != 1 {
		t.Fatal("scope shutdown retried or dispatched input", err, calls, opens)
	}
}
