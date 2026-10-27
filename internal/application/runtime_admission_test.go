package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestLeaseAdmissionCancellationCannotChangeAuthorityOrSequence(t *testing.T) {
	type operation struct {
		name  string
		setup func(*SessionRuntime) error
		run   func(context.Context, *SessionRuntime) error
	}
	begin := func(r *SessionRuntime) error {
		_, err := r.BeginOutbound(context.Background(), "s", "e")
		return err
	}
	prepare := func(r *SessionRuntime) error {
		_, err := r.PrepareInbound(context.Background(), "s", "l", "e")
		return err
	}
	operations := []operation{
		{name: "begin", run: func(ctx context.Context, r *SessionRuntime) error {
			_, err := r.BeginOutbound(ctx, "s", "e")
			return err
		}},
		{name: "grant", setup: begin, run: func(ctx context.Context, r *SessionRuntime) error {
			_, err := r.GrantOutbound(ctx, "s", "l", "e", time.Now())
			return err
		}},
		{name: "prepare", run: func(ctx context.Context, r *SessionRuntime) error {
			_, err := r.PrepareInbound(ctx, "s", "l", "e")
			return err
		}},
		{name: "commit", setup: prepare, run: func(ctx context.Context, r *SessionRuntime) error {
			_, err := r.CommitInbound(ctx, "s", "l", "e", time.Now())
			return err
		}},
		{name: "envelope", setup: func(r *SessionRuntime) error {
			if err := begin(r); err != nil {
				return err
			}
			_, err := r.GrantOutbound(context.Background(), "s", "l", "e", time.Now())
			return err
		}, run: func(ctx context.Context, r *SessionRuntime) error {
			envelope, err := r.NextOutboundEnvelope(ctx)
			if err != nil && envelope != (control.InputEnvelope{}) {
				t.Error("canceled allocation returned an envelope")
			}
			return err
		}},
		{name: "validate", setup: begin, run: func(ctx context.Context, r *SessionRuntime) error {
			return r.ValidateOutboundPreparation(ctx, "s", "l", "e", time.Now())
		}},
	}
	for _, op := range operations {
		for _, waiting := range []bool{false, true} {
			mode := "already-canceled"
			if waiting {
				mode = "deadline-during-lock"
			}
			t.Run(op.name+"/"+mode, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					executor := &recordingEffectExecutor{}
					r, err := NewSessionRuntime(executor, &recordingInputPort{})
					if err != nil {
						t.Fatal(err)
					}
					if op.setup != nil {
						if err := op.setup(r); err != nil {
							t.Fatal(err)
						}
					}
					before, watch := r.Snapshot(), r.watch.Load()
					executor.calls = nil
					ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
					defer cancel()
					var ownerDone chan struct{}
					wantErr, wantWait := error(context.Canceled), time.Duration(0)
					if waiting {
						wantErr, wantWait = context.DeadlineExceeded, 25*time.Millisecond
						r.mu.Lock()
						ownerDone = make(chan struct{})
						go func() {
							time.Sleep(500 * time.Millisecond)
							r.mu.Unlock()
							close(ownerDone)
						}()
					} else {
						cancel()
					}
					started := time.Now()
					err = op.run(ctx, r)
					elapsed := time.Since(started)
					if ownerDone != nil {
						<-ownerDone
					}
					if !errors.Is(err, wantErr) || elapsed != wantWait {
						t.Errorf("admission ignored cancellation: err=%v wait=%v want=%v", err, elapsed, wantWait)
					}
					if !reflect.DeepEqual(before, r.Snapshot()) || r.watch.Load() != watch || len(executor.calls) != 0 {
						t.Error("canceled admission changed state, liveness, sequence, watch, or native effects")
					}
				})
			})
		}
	}
}
