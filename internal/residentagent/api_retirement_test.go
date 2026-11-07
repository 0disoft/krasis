package residentagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type retirementAPI struct {
	started  chan struct{}
	closed   chan struct{}
	serving  context.Context
	failure  error
	closeErr error
	once     sync.Once
}

func (s *retirementAPI) Run(ctx context.Context) error {
	s.serving = ctx
	close(s.started)
	if s.failure != nil {
		return s.failure
	}
	select {
	case <-ctx.Done():
	case <-s.closed:
	}
	return s.Close(ctx)
}

func (s *retirementAPI) Close(context.Context) error {
	s.once.Do(func() { close(s.closed) })
	return s.closeErr
}

func TestAPIOutlivesInputRetirement(t *testing.T) {
	for _, retirementFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "failed"}[retirementFailure], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := &retirementAPI{started: make(chan struct{}), closed: make(chan struct{})}
			release := make(chan struct{})
			defer close(release)
			entered := make(chan struct{})
			failure := errors.New("native retirement failed")
			done := make(chan error, 1)
			go func() {
				done <- serveUntilInputRetired(ctx, server, func(cleanup context.Context) error {
					close(entered)
					select {
					case <-release:
					case <-cleanup.Done():
						return cleanup.Err()
					}
					if retirementFailure {
						return failure
					}
					return nil
				})
			}()
			<-server.started
			cancel()
			<-entered
			if server.serving.Err() != nil {
				t.Fatal("API lifetime was canceled before native retirement finished")
			}
			select {
			case <-server.closed:
				t.Fatal("API closed while native retirement still needed agent presence")
			default:
			}
			release <- struct{}{}
			select {
			case err := <-done:
				if retirementFailure && !errors.Is(err, failure) || !retirementFailure && err != nil {
					t.Fatalf("retirement error lost: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("API owner did not join after retirement")
			}
			select {
			case <-server.closed:
			default:
				t.Fatal("API not closed after native retirement")
			}
		})
	}
}

func TestAPIFailureStillRetiresInputAndPreservesAllErrors(t *testing.T) {
	serveErr, closeErr := errors.Join(context.Canceled, errors.New("serve failed")), errors.New("close failed")
	retireErr := errors.Join(context.Canceled, errors.New("retire failed"))
	server := &retirementAPI{started: make(chan struct{}), closed: make(chan struct{}), failure: serveErr, closeErr: closeErr}
	retired := false
	err := serveUntilInputRetired(context.Background(), server, func(ctx context.Context) error {
		retired = true
		if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
			t.Fatal("retirement has no independent live cleanup budget")
		}
		return retireErr
	})
	if !retired || !errors.Is(err, serveErr) || !errors.Is(err, closeErr) || !errors.Is(err, retireErr) {
		t.Fatalf("failure path lost ownership or errors: %v", err)
	}
}
