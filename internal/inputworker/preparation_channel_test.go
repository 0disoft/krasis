package inputworker

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestPreparationPeerDisconnectCancelsBuildAndJoinsCleanup(t *testing.T) {
	for _, invalidData := range []bool{false, true} {
		agent, service := net.Pipe()
		entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- watchPreparationPeer(context.Background(), service, func(run context.Context) error {
				close(entered)
				<-run.Done()
				close(canceled)
				<-release
				return run.Err()
			})
		}()
		<-entered
		if invalidData {
			_, _ = agent.Write([]byte{1})
		} else {
			_ = agent.Close()
		}
		select {
		case <-canceled:
		case <-time.After(time.Second):
			_ = agent.Close()
			_ = service.Close()
			close(release)
			t.Fatal("abandoned preparation did not cancel native builder")
		}
		select {
		case <-done:
			t.Fatal("cancellation bypassed builder cleanup")
		default:
		}
		close(release)
		if err := <-done; !errors.Is(err, context.Canceled) || errors.Is(err, ErrRecovery) {
			t.Fatal("clean cancellation became uncertain", err)
		}
		_ = agent.Close()
		_ = service.Close()
	}
}

func TestCompletedPreparationJoinsWatcherBeforeReceiptAndPreservesFailure(t *testing.T) {
	for _, failure := range []error{nil, ErrRecovery} {
		agent, service := net.Pipe()
		done := make(chan error, 1)
		go func() {
			err := watchPreparationPeer(context.Background(), service, func(context.Context) error { return failure })
			if failure == nil && err == nil {
				err = writeFrame(service, preparationReceipt[:])
			}
			done <- err
		}()
		if failure == nil {
			var receipt [8]byte
			_ = agent.SetReadDeadline(time.Now().Add(time.Second))
			if _, err := io.ReadFull(agent, receipt[:]); err != nil || receipt != preparationReceipt {
				t.Fatal("watcher prevented receipt", err)
			}
		}
		if err := <-done; !errors.Is(err, failure) {
			t.Fatal("build error hidden or successful build canceled", err)
		}
		_ = agent.Close()
		_ = service.Close()
	}
}
