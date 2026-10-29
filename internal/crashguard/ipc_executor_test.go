package crashguard

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestIPCExecutorReusesReplyOwnerAndJoinsShutdown(t *testing.T) {
	p := closeTestProcess(t)
	for i := 0; i < 1000; i++ {
		if err := p.invoke(func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		if p.executor == nil {
			t.Fatal("input used a per-command executor")
		}
	}
	owner := p.executor
	close(p.done)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-owner.done:
	default:
		t.Fatal("retirement published before reusable IPC owner exited")
	}
}

func TestIPCExecutorDoesNotDispatchAfterExit(t *testing.T) {
	exited := make(chan struct{})
	e := newIPCExecutor(exited)
	close(exited)
	<-e.done
	var active sync.WaitGroup
	if err := <-e.start(context.Background(), func() error {
		t.Error("retired executor ran an operation")
		return nil
	}, &active); !errors.Is(err, ErrUnexpectedExit) {
		t.Fatal(err)
	}
	active.Wait()
}

func BenchmarkIPCExecution(b *testing.B) {
	b.Run("per-command", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			result := make(chan error, 1)
			go func() { result <- nil }()
			<-result
		}
	})
	b.Run("reused", func(b *testing.B) {
		e := newIPCExecutor(nil)
		defer func() { close(e.stop); <-e.done }()
		var active sync.WaitGroup
		operation := func() error { return nil }
		b.ReportAllocs()
		for b.Loop() {
			<-e.start(context.Background(), operation, &active)
		}
		active.Wait()
	})
}
