package x11desktop

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/cleanupresult"
)

type closeEvidenceWire struct {
	net.Conn
	calls atomic.Int32
	err   error
}

func TestX11CanceledSetupKeepsFirstRetirementEvidence(t *testing.T) {
	failure := errors.New("startup close failed")
	for _, closeErr := range []error{nil, net.ErrClosed, failure} {
		wire := &closeEvidenceWire{err: closeErr}
		conn := &connection{wire: wire}
		_ = conn.close() // cancellation wins before initialization returns
		cause := errors.Join(context.Canceled, io.EOF)
		err := retiredOpenFailure(conn, cause)
		if !errors.Is(err, context.Canceled) || !errors.Is(err, io.EOF) || wire.calls.Load() != 1 {
			t.Fatal("setup failure/cancellation or single close ownership lost", err)
		}
		if cleanupresult.Confirmed(err) != (closeErr != failure) {
			t.Fatal("unverified startup retirement accepted", err)
		}
		if closeErr == failure && !errors.Is(err, failure) {
			t.Fatal("startup swallowed original close failure", err)
		}
	}
}

func (w *closeEvidenceWire) Close() error              { w.calls.Add(1); return w.err }
func (*closeEvidenceWire) SetDeadline(time.Time) error { return nil }

func TestX11CloseRetainsRealFailureAndSerializesOwnership(t *testing.T) {
	failure := errors.New("native close failed")
	for _, cause := range []error{nil, net.ErrClosed, failure} {
		wire := &closeEvidenceWire{err: cause}
		conn := &connection{wire: wire}
		d := &Desktop{c: conn}
		want := cause
		if cause == net.ErrClosed {
			want = nil
		}
		var joined sync.WaitGroup
		for range 8 {
			joined.Add(1)
			go func() {
				defer joined.Done()
				if err := d.Close(); err != want {
					t.Errorf("close evidence=%v want=%v", err, want)
				}
			}()
		}
		joined.Wait()
		if err := conn.close(); err != want || wire.calls.Load() != 1 {
			t.Fatal("close lost ownership/error", err, wire.calls.Load())
		}
	}
}

func TestX11RequestFailureDoesNotHideSubsequentCloseFailure(t *testing.T) {
	failure := errors.New("native close failed")
	wire := &closeEvidenceWire{err: failure}
	conn := &connection{wire: wire}
	if err := conn.do(context.Background(), func() error { return io.EOF }); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	d := &Desktop{c: conn}
	if err := d.Close(); !errors.Is(err, failure) || wire.calls.Load() != 1 {
		t.Fatal("operation failure masked retirement failure", err)
	}
}
