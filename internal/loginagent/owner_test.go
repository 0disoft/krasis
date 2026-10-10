package loginagent

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type testWorker struct {
	done    chan struct{}
	ctx     context.Context
	stopErr error
	once    sync.Once
	stops   int
}

func (w *testWorker) Done() <-chan struct{} { return w.done }
func (w *testWorker) Stop(ctx context.Context) error {
	w.stops++
	if ctx.Err() != nil || w.ctx.Err() == nil {
		return errors.New("cleanup lacks independent context or lifetime cancellation")
	}
	if w.stopErr != nil {
		return w.stopErr
	}
	w.once.Do(func() { close(w.done) })
	return nil
}

func enabled() Selection { return Selection{Enabled: true, Profile: [32]byte{1}} }

func TestLoginRetiresWorkerAndLockNeverRestartsIt(t *testing.T) {
	console := Console{Session: 1, State: BeforeLogin}
	var workers []*testWorker
	owner, err := New(Dependencies{
		Read: func() (Console, error) { return console, nil },
		Start: func(ctx context.Context, session uint32, profile [32]byte) (Worker, error) {
			if session != console.Session || profile != enabled().Profile {
				t.Fatal("start target was changed")
			}
			worker := &testWorker{ctx: ctx, done: make(chan struct{})}
			workers = append(workers, worker)
			return worker, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	for range 5 {
		if err := owner.Reconcile(context.Background(), enabled()); err != nil {
			t.Fatal(err)
		}
	}
	if len(workers) != 1 {
		t.Fatal("polling duplicated worker")
	}
	console.State = UserSession
	for range 5 {
		if err := owner.Reconcile(context.Background(), enabled()); err != nil {
			t.Fatal(err)
		}
	}
	if len(workers) != 1 || workers[0].stops != 1 {
		t.Fatal("login/lock ownership transition failed")
	}
	console = Console{Session: 2, State: BeforeLogin}
	if err := owner.Reconcile(context.Background(), enabled()); err != nil {
		t.Fatal(err)
	}
	if len(workers) != 2 {
		t.Fatal("new pre-login console did not get its own worker")
	}
}

func TestObservationFailureStopsWithoutTreatingItAsLogout(t *testing.T) {
	readErr := error(nil)
	var worker *testWorker
	starts := 0
	owner, _ := New(Dependencies{
		Read: func() (Console, error) { return Console{Session: 1, State: BeforeLogin}, readErr },
		Start: func(ctx context.Context, _ uint32, _ [32]byte) (Worker, error) {
			starts++
			worker = &testWorker{ctx: ctx, done: make(chan struct{})}
			return worker, nil
		},
	})
	defer owner.Close()
	if err := owner.Reconcile(context.Background(), enabled()); err != nil {
		t.Fatal(err)
	}
	readErr = errors.New("query failed")
	for range 3 {
		if err := owner.Reconcile(context.Background(), enabled()); !errors.Is(err, ErrObservation) {
			t.Fatal(err)
		}
	}
	if starts != 1 || worker.stops != 1 {
		t.Fatal("failed observation kept/restarted worker")
	}
}

func TestRetirementFailureFencesReplacement(t *testing.T) {
	console := Console{Session: 1, State: BeforeLogin}
	starts := 0
	owner, _ := New(Dependencies{
		Read: func() (Console, error) { return console, nil },
		Start: func(ctx context.Context, _ uint32, _ [32]byte) (Worker, error) {
			starts++
			return &testWorker{ctx: ctx, done: make(chan struct{}), stopErr: errors.New("key release unconfirmed")}, nil
		},
	})
	if err := owner.Reconcile(context.Background(), enabled()); err != nil {
		t.Fatal(err)
	}
	console.Session = 2
	if err := owner.Reconcile(context.Background(), enabled()); !errors.Is(err, ErrRecovery) {
		t.Fatal(err)
	}
	selection := enabled()
	selection.Profile[0]++
	if err := owner.Reconcile(context.Background(), selection); !errors.Is(err, ErrRecovery) {
		t.Fatal(err)
	}
	if err := owner.Close(); !errors.Is(err, ErrRecovery) {
		t.Fatal(err)
	}
	if starts != 1 {
		t.Fatal("replacement bypassed unconfirmed retirement")
	}
}

func TestFailedStartAndUnexpectedExitDoNotLoop(t *testing.T) {
	for _, startupFailure := range []bool{true, false} {
		t.Run(map[bool]string{true: "start", false: "exit"}[startupFailure], func(t *testing.T) {
			starts := 0
			var worker *testWorker
			owner, _ := New(Dependencies{
				Read: func() (Console, error) { return Console{Session: 1, State: BeforeLogin}, nil },
				Start: func(ctx context.Context, _ uint32, _ [32]byte) (Worker, error) {
					starts++
					if startupFailure {
						return nil, errors.New("launch failed")
					}
					worker = &testWorker{ctx: ctx, done: make(chan struct{})}
					return worker, nil
				},
			})
			defer owner.Close()
			err := owner.Reconcile(context.Background(), enabled())
			if startupFailure && !errors.Is(err, ErrStart) || !startupFailure && err != nil {
				t.Fatal(err)
			}
			if worker != nil {
				worker.once.Do(func() { close(worker.done) })
			}
			for range 4 {
				if err := owner.Reconcile(context.Background(), enabled()); !errors.Is(err, ErrStart) {
					t.Fatal(err)
				}
			}
			if starts != 1 {
				t.Fatal("polling retried a failed worker")
			}
		})
	}
}

func TestLaunchLogonRaceAndCancellationRetireWorker(t *testing.T) {
	for _, cancelRequest := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancel", false: "logon"}[cancelRequest], func(t *testing.T) {
			console := Console{Session: 1, State: BeforeLogin}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var worker *testWorker
			owner, _ := New(Dependencies{
				Read: func() (Console, error) { return console, nil },
				Start: func(lifetime context.Context, _ uint32, _ [32]byte) (Worker, error) {
					worker = &testWorker{ctx: lifetime, done: make(chan struct{})}
					if cancelRequest {
						cancel()
					} else {
						console.State = UserSession
					}
					return worker, nil
				},
			})
			defer owner.Close()
			if err := owner.Reconcile(ctx, enabled()); err == nil {
				t.Fatal("race adopted worker")
			}
			if worker.stops != 1 || owner.worker != nil {
				t.Fatal("race did not retire worker")
			}
		})
	}
}

func TestDisableAndCallerCancellationCleanUp(t *testing.T) {
	for _, cancellation := range []bool{false, true} {
		var worker *testWorker
		owner, _ := New(Dependencies{
			Read: func() (Console, error) { return Console{Session: 1, State: BeforeLogin}, nil },
			Start: func(ctx context.Context, _ uint32, _ [32]byte) (Worker, error) {
				worker = &testWorker{ctx: ctx, done: make(chan struct{})}
				return worker, nil
			},
		})
		if err := owner.Reconcile(context.Background(), enabled()); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		selection := Selection{}
		if cancellation {
			cancel()
			selection = enabled()
		}
		err := owner.Reconcile(ctx, selection)
		cancel()
		if cancellation && !errors.Is(err, context.Canceled) || !cancellation && err != nil {
			t.Fatal(err)
		}
		if worker.stops != 1 || owner.worker != nil {
			t.Fatal("worker survived revocation")
		}
		if err := owner.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompletedRequestDoesNotCancelAdoptedWorker(t *testing.T) {
	var worker *testWorker
	owner, _ := New(Dependencies{
		Read: func() (Console, error) { return Console{Session: 1, State: BeforeLogin}, nil },
		Start: func(ctx context.Context, _ uint32, _ [32]byte) (Worker, error) {
			worker = &testWorker{ctx: ctx, done: make(chan struct{})}
			return worker, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	if err := owner.Reconcile(ctx, enabled()); err != nil {
		t.Fatal(err)
	}
	cancel()
	if worker.ctx.Err() != nil {
		t.Fatal("request context owned worker lifetime")
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if worker.stops != 1 {
		t.Fatal("close did not retire worker")
	}
}

func TestGrantReplacementRetiresBeforeStarting(t *testing.T) {
	var previous *testWorker
	starts := 0
	owner, _ := New(Dependencies{
		Read: func() (Console, error) { return Console{Session: 1, State: BeforeLogin}, nil },
		Start: func(ctx context.Context, _ uint32, _ [32]byte) (Worker, error) {
			if previous != nil {
				select {
				case <-previous.done:
				default:
					t.Fatal("replacement overlapped previous owner")
				}
			}
			starts++
			previous = &testWorker{ctx: ctx, done: make(chan struct{})}
			return previous, nil
		},
	})
	defer owner.Close()
	if err := owner.Reconcile(context.Background(), enabled()); err != nil {
		t.Fatal(err)
	}
	selection := enabled()
	selection.Profile[0]++
	if err := owner.Reconcile(context.Background(), selection); err != nil {
		t.Fatal(err)
	}
	if starts != 2 {
		t.Fatal("replacement not started")
	}
}
