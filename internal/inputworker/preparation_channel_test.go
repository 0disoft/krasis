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

type contextPreparationReader struct {
	net.Conn
	entered, canceled, release chan struct{}
}

func (reader *contextPreparationReader) Read([]byte) (int, error) {
	panic("context-aware preparation fell back to polling Read")
}

func (reader *contextPreparationReader) readContext(ctx context.Context, _ []byte) (int, error) {
	close(reader.entered)
	<-ctx.Done()
	close(reader.canceled)
	<-reader.release
	return 0, ctx.Err()
}

func TestCompletedPreparationCancelsExactWatcherAndJoinsItBeforeReturning(t *testing.T) {
	for _, failure := range []error{nil, ErrRecovery} {
		agent, service := net.Pipe()
		reader := &contextPreparationReader{Conn: service, entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
		done := make(chan error, 1)
		go func() {
			done <- watchPreparationPeer(context.Background(), reader, func(context.Context) error {
				<-reader.entered
				return failure
			})
		}()
		select {
		case <-reader.canceled:
		case <-time.After(time.Second):
			t.Fatal("completed preparation did not cancel its exact reader")
		}
		select {
		case <-done:
			t.Fatal("preparation returned with watcher still using its buffer")
		default:
		}
		close(reader.release)
		if err := <-done; !errors.Is(err, failure) || errors.Is(err, context.Canceled) {
			t.Fatal("watcher cancellation changed build result", err)
		}
		_ = agent.Close()
		_ = service.Close()
	}
}

type completedPreparationReader struct {
	net.Conn

	entered chan struct{}
}

func (reader *completedPreparationReader) readContext(ctx context.Context, data []byte) (int, error) {
	close(reader.entered)
	<-ctx.Done()
	// A kernel read can complete before its cancellation is observed. Its
	// bytes remain authoritative even if preparation has just finished.
	data[0] = 1
	return 1, nil
}

func TestCompletedPreparationStillRejectsCompletedExtraInput(t *testing.T) {
	agent, service := net.Pipe()
	defer agent.Close()
	defer service.Close()
	reader := &completedPreparationReader{Conn: service, entered: make(chan struct{})}
	err := watchPreparationPeer(context.Background(), reader, func(context.Context) error {
		<-reader.entered
		return nil
	})
	if !errors.Is(err, ErrProtocol) {
		t.Fatal("completed extra input was acknowledged as successful preparation", err)
	}
}
