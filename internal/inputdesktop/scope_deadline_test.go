package inputdesktop

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"
)

// Model a delayed deadline callback: the deadline passes while Err stays nil.
type undeliveredDesktopDeadline struct {
	context.Context
	deadline time.Time
}

func (c undeliveredDesktopDeadline) Deadline() (time.Time, bool) { return c.deadline, true }

func TestScopeDoesNotDispatchAfterDesktopQueryConsumesDeadline(t *testing.T) {
	for _, owner := range []bool{false, true} {
		t.Run(fmt.Sprintf("owner=%t", owner), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, root := context.Context(context.Background()), context.Context(context.Background())
				deadline := undeliveredDesktopDeadline{Context: context.Background(), deadline: time.Now().Add(50 * time.Millisecond)}
				if owner {
					root = deadline
				} else {
					ctx = deadline
				}
				b := &interactiveBinding{authorize: func() error { return nil }, current: &testBinding{activeFn: func() {
					time.Sleep(50 * time.Millisecond)
				}}}
				scope := &Scope{root: root, desktop: b, owner: 1, thread: func() uintptr { return 1 }}
				calls := 0
				err := scope.Do(ctx, func() error { calls++; return nil })
				if !errors.Is(err, context.DeadlineExceeded) || calls != 0 {
					t.Fatalf("expired input dispatched: err=%v calls=%d", err, calls)
				}
			})
		})
	}
}

func newDeadlineTestScope() *Scope {
	return &Scope{root: context.Background(), desktop: &testBinding{}, owner: 1, thread: func() uintptr { return 1 }}
}

func TestScopeDeadlineAdmissionBoundaries(t *testing.T) {
	for _, source := range []string{"caller", "owner", "abort"} {
		for _, duringQuery := range []bool{false, true} {
			for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
				t.Run(fmt.Sprintf("%s/during-query=%t/%s", source, duringQuery, delta), func(t *testing.T) {
					scope := newDeadlineTestScope()
					deadline := time.Now().Add(time.Hour)
					now := deadline.Add(delta)
					if duringQuery {
						now = deadline.Add(-time.Second)
					}
					ctx := context.Context(context.Background())
					opt := scopeOperationOptions{now: func() time.Time { return now }}
					bounded := undeliveredDesktopDeadline{Context: context.Background(), deadline: deadline}
					switch source {
					case "caller":
						ctx = bounded
					case "owner":
						scope.root = bounded
					case "abort":
						opt.signal = bounded
					}
					queries, inputs := 0, 0
					opt.active = func(context.Context, context.Context) error {
						queries++
						now = deadline.Add(delta)
						return nil
					}
					err := scope.do(ctx, func() error { inputs++; return nil }, opt)
					wantQueries, wantInputs := 1, 1
					if delta >= 0 {
						wantInputs = 0
						if !duringQuery {
							wantQueries = 0
						}
						if !errors.Is(err, context.DeadlineExceeded) {
							t.Fatal("deadline admitted input", err)
						}
					} else if err != nil {
						t.Fatal("live input rejected", err)
					}
					if queries != wantQueries || inputs != wantInputs {
						t.Fatalf("queries=%d inputs=%d; want %d/%d", queries, inputs, wantQueries, wantInputs)
					}
				})
			}
		}
	}
}

func TestScopeCancellationFencesDesktopIOAndInput(t *testing.T) {
	for _, source := range []string{"caller", "owner", "abort"} {
		for _, duringQuery := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/during-query=%t", source, duringQuery), func(t *testing.T) {
				scope := newDeadlineTestScope()
				signal, cancel := context.WithCancel(context.Background())
				defer cancel()
				ctx := context.Context(context.Background())
				opt := scopeOperationOptions{}
				switch source {
				case "caller":
					ctx = signal
				case "owner":
					scope.root = signal
				case "abort":
					opt.signal = signal
				}
				queries, inputs := 0, 0
				opt.active = func(context.Context, context.Context) error { queries++; cancel(); return nil }
				if !duringQuery {
					cancel()
				}
				err := scope.do(ctx, func() error { inputs++; return nil }, opt)
				wantQueries := 0
				if duringQuery {
					wantQueries = 1
				}
				if !errors.Is(err, context.Canceled) || queries != wantQueries || inputs != 0 {
					t.Fatalf("canceled input: err=%v queries=%d inputs=%d", err, queries, inputs)
				}
			})
		}
	}
}

func TestScopeAbortInterruptsPendingDesktopCheck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		scope := newDeadlineTestScope()
		signal, abort := context.WithCancel(context.Background())
		defer abort()
		inputs := 0
		err := scope.do(context.Background(), func() error { inputs++; return nil }, scopeOperationOptions{
			signal: signal,
			active: func(ctx, _ context.Context) error {
				abort()
				<-ctx.Done()
				return nil // A completed native query can race its abort receipt.
			},
		})
		if !errors.Is(err, context.Canceled) || inputs != 0 {
			t.Fatal("abort did not fence dispatch", err, inputs)
		}
	})
}

func TestScopePreservesNativeFailureDuringCancellation(t *testing.T) {
	scope := newDeadlineTestScope()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failure := errors.New("native cleanup failed")
	inputs := 0
	err := scope.do(ctx, func() error { inputs++; return nil }, scopeOperationOptions{
		active: func(context.Context, context.Context) error {
			cancel()
			return errors.Join(ErrCleanup, failure)
		},
	})
	for _, want := range []error{ErrInactive, ErrCleanup, failure, context.Canceled} {
		if !errors.Is(err, want) {
			t.Fatalf("lost %v: %v", want, err)
		}
	}
	if inputs != 0 {
		t.Fatal("cleanup failure admitted input")
	}
}

func TestScopeStartedInputKeepsOriginalOutcome(t *testing.T) {
	for _, failure := range []error{nil, errors.New("injection outcome unknown")} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			scope := newDeadlineTestScope()
			deadline := time.Now().Add(time.Hour)
			now := deadline.Add(-time.Nanosecond)
			caller, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := undeliveredDesktopDeadline{Context: caller, deadline: deadline}
			inputs := 0
			err := scope.do(ctx, func() error {
				inputs++
				now = deadline.Add(time.Nanosecond)
				cancel()
				return failure
			}, scopeOperationOptions{now: func() time.Time { return now }})
			if err != failure || inputs != 1 {
				t.Fatal("started input was retried or its outcome rewritten", err, inputs)
			}
		})
	}
}
