package residentagent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestActualEntryCancelsPreparationButWaitsForCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered, canceled, release, prepared := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	connected, opened := make(chan struct{}), make(chan error, 1)
	var once sync.Once
	input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), prepare: func(run context.Context) error {
		close(entered)
		<-run.Done()
		close(canceled)
		<-release // Native cleanup must be joined, not bypassed by cancellation.
		return run.Err()
	}, connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
		close(connected)
		return &testPrivilegedClient{}, nil
	}}
	go func() { _, _ = input.prepareIdle(ctx); close(prepared) }()
	<-entered
	go func() {
		opened <- input.open(ctx, application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now())
	}()
	t.Cleanup(func() { cancel(); once.Do(func() { close(release) }); <-prepared; input.shutdown() })
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("actual entry waited for optional preparation instead of canceling it")
	}
	select {
	case <-connected:
		t.Fatal("entry bypassed preparation cleanup")
	default:
	}
	once.Do(func() { close(release) })
	select {
	case err := <-opened:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("entry did not resume after preparation cleanup")
	}
}
