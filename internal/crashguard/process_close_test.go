package crashguard

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"
)

type closeTestPipe struct{ bytes.Buffer }

func (*closeTestPipe) Close() error { return nil }

func closeTestProcess(t *testing.T) *Process {
	t.Helper()
	var ack bytes.Buffer
	if err := writeAck(&ack, operationDisarm, statusOK); err != nil {
		t.Fatal(err)
	}
	in := &closeTestPipe{}
	client, err := NewClient(in, &ack)
	if err != nil {
		t.Fatal(err)
	}
	return &Process{client: client, stdin: in, stdout: io.NopCloser(&bytes.Buffer{}), done: make(chan struct{}), stderr: &limitedBuffer{}}
}

func TestCloseBoundsFailedKillAndRetainsUnconfirmedExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := closeTestProcess(t)
		denied := errors.New("termination denied")
		kills := 0
		p.kill = func() error { kills++; return denied }
		started := time.Now()
		err := p.Close()
		var pending *ExitPending
		if !errors.Is(err, ErrExitTimeout) || !errors.Is(err, denied) || !errors.As(err, &pending) || pending.Done != p.closeDone || time.Since(started) != 2*processIPCTimeout {
			t.Fatal(err, time.Since(started))
		}
		select {
		case <-p.Done():
			t.Fatal("timeout fabricated an exit acknowledgement")
		default:
		}
		<-p.requestAbort()
		if kills != 1 {
			t.Fatal("termination retried", kills)
		}
		close(p.done)
	})
}

func TestCloseObservesExitAfterTerminationWithinRemainingBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := closeTestProcess(t)
		p.kill = func() error {
			go func() { time.Sleep(time.Millisecond); close(p.done) }()
			return nil
		}
		err := p.Close()
		var pending *ExitPending
		if !errors.Is(err, ErrExitTimeout) || errors.As(err, &pending) {
			t.Fatal(err)
		}
		select {
		case <-p.Done():
		default:
			t.Fatal("exit not confirmed")
		}
	})
}

func TestCloseSuccessfulExitDoesNotTerminate(t *testing.T) {
	p := closeTestProcess(t)
	p.kill = func() error { t.Fatal("terminated an exited guard"); return nil }
	close(p.done)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}
