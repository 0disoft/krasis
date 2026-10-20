package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestSlowInputAuthorizationDoesNotStarveWorkerHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lease := testLease()
		conn, workerDone := startWorker(t, context.Background(), &recordingInput{}, lease)
		stream := &admissionObservedConn{Conn: conn}
		channel := clientChannel(stream, authorityService)
		client, err := NewClient(context.Background(), channel, lease)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		entered, release := make(chan struct{}), make(chan struct{})
		var first atomic.Bool
		channel.authority.checkInputObserved = func(context.Context) error {
			if !first.Swap(true) {
				close(entered)
				<-release
			}
			return nil
		}
		channel.authority.checkPresent = func() error { return nil }
		inputDone := make(chan error, 1)
		go func() { inputDone <- client.Move(context.Background(), 1, 0) }()
		<-entered
		time.Sleep(time.Second)
		firstHeartbeat := client.Heartbeat(context.Background())
		time.Sleep(500 * time.Millisecond)
		secondHeartbeat := client.Heartbeat(context.Background())
		heartbeatWrites := stream.writes.Load()
		time.Sleep(time.Second)
		// The caller must already have returned although native code is stuck.
		err = <-inputDone
		if err := client.Close(); err != ErrAuthority {
			t.Fatal("unconfirmed query retirement reported cleanup success", err)
		}
		close(release)
		synctest.Wait()
		certainty, ok := application.InjectionCertaintyOf(err)
		if firstHeartbeat != nil || secondHeartbeat != nil || heartbeatWrites != 2 || !errors.Is(err, context.DeadlineExceeded) || !ok || certainty != application.InjectionNotApplied {
			t.Fatal("unsent native authorization starved maintenance or became transmitted input", firstHeartbeat, secondHeartbeat, heartbeatWrites, err)
		}
		if err := client.Move(context.Background(), 2, 0); err == nil || stream.writes.Load() != 2 {
			t.Fatal("late native success revived an expired proof", err)
		}
		_ = client.Close()
		if result := await(t, workerDone); result.err != nil || result.report.Completed != 0 {
			t.Fatal("heartbeat changed worker input completion or cleanup", result)
		}
	})
}

func TestContendedInputAdmissionDiscardsEarlierAuthorization(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lease := testLease()
		conn, workerDone := startWorker(t, context.Background(), &recordingInput{}, lease)
		stream := &admissionObservedConn{Conn: conn}
		channel := clientChannel(stream, authorityService)
		client, err := NewClient(context.Background(), channel, lease)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		var permitted atomic.Bool
		permitted.Store(true)
		var checks atomic.Int32
		channel.authority.checkInputObserved = func(context.Context) error {
			checks.Add(1)
			if !permitted.Load() {
				return ErrAuthority
			}
			return nil
		}
		channel.authority.checkPresent = func() error { return nil }
		client.mu.Lock()
		done := make(chan error, 1)
		go func() { done <- client.Move(context.Background(), 1, 0) }()
		synctest.Wait()
		permitted.Store(false)
		client.mu.Unlock()
		err = <-done
		if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionNotApplied || checks.Load() != 2 || stream.writes.Load() != 0 {
			t.Fatal("authorization survived contention or input reached the worker", err, checks.Load(), stream.writes.Load())
		}
		_ = client.Close()
		if result := await(t, workerDone); result.err != nil || result.report.Completed != 0 {
			t.Fatal("rejected admission bypassed worker cleanup", result)
		}
	})
}
