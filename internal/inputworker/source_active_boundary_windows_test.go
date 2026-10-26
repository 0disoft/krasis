//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/wininput"
)

type activeBoundarySink struct{ total atomic.Int64 }

func (s *activeBoundarySink) SendAction(_ context.Context, a control.InputAction, current func() bool) error {
	if current() {
		s.total.Add(int64(a.DeltaX))
	}
	return nil
}
func (*activeBoundarySink) Interrupt(error) error             { return nil }
func (*activeBoundarySink) Fail(context.Context, error) error { return nil }

func TestActiveSourceMovesDoNotDuplicateBoundaryTraffic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &activeBoundarySink{}
		session, err := wininput.NewSourceSession(1024, sink)
		if err != nil {
			t.Fatal(err)
		}
		run, cancel := context.WithCancel(context.Background())
		ready, done := make(chan struct{}), make(chan error, 1)
		go func() { done <- session.RunWithReady(run, func() { close(ready) }) }()
		<-ready
		defer func() { cancel(); <-done }()
		q := &sourcePointerQueue{events: make(chan sourcePacket, 64), session: session}
		if err := activateSourceSession(session); err != nil {
			t.Fatal(err)
		}
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
		for range 1000 {
			if err := q.Observe(move, 999); err != nil || !session.Offer(move) {
				t.Fatal("active input was rejected", err)
			}
		}
		synctest.Wait()
		if len(q.events) != 0 || sink.total.Load() != 1000 {
			t.Fatalf("active movement duplicated advisory traffic or lost input: boundaries=%d delta=%d", len(q.events), sink.total.Load())
		}
		session.Deactivate()
		detector, _ := control.NewBoundaryDetector(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 1000, RearmDistance: 10})
		_, _, _ = detector.Observe(move, 999) // Previously crossed edge is disarmed.
		for i, x := range []int32{500, 999} {
			if err := q.Observe(move, x); err != nil {
				t.Fatal(err)
			}
			select {
			case p := <-q.events:
				_, fired, err := detector.Observe(p.action, p.x)
				if err != nil || fired != (i == 1) || p.x != x {
					t.Fatal("local rearm/edge order changed after return", p, fired, err)
				}
			default:
				t.Fatal("local boundary observation did not resume")
			}
		}
		if err := activateSourceSession(session); err != nil {
			t.Fatal(err)
		}
		for _, code := range []uint32{0x1D, 0x38, 0x2A} {
			if err := q.Observe(control.InputAction{Kind: control.InputKeyDown, Code: code}, 0); err != nil {
				t.Fatal(err)
			}
		}
		if err := q.Observe(control.InputAction{Kind: control.InputKeyDown, Code: 0x58}, 0); !errors.Is(err, wininput.ErrEmergencyRelease) || session.Active() {
			t.Fatal("active advisory suppression hid emergency release", err)
		}
	})
}

func TestBoundaryBatchIsDiscardedWhenCaptureActivatesDuringProof(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		stream := &admissionObservedConn{Conn: pipe}
		w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}, sequence: 17}
		defer w.channel.Close()
		w.started.Store(true)
		entered, release := make(chan struct{}), make(chan struct{})
		w.channel.authority.checkPresent = func() error { return nil }
		w.channel.authority.checkInput = func() error { close(entered); <-release; return nil }
		done := make(chan error, 1)
		go func() { done <- w.sendBoundaries([]sourcePacket{boundaryBatchPacket(999)}) }()
		<-entered
		w.epoch.Store(23)
		close(release)
		if err := <-done; err != nil || stream.writes.Load() != 0 || w.sequence != 17 || w.broken {
			t.Fatal("pre-activation advisory batch held the active wire", err, stream.writes.Load(), w.sequence)
		}
	})
}

func TestActiveBoundaryBatchDoesNotQueryAuthorityOrHideInvalidInput(t *testing.T) {
	server, pipe := net.Pipe()
	defer server.Close()
	stream := &admissionObservedConn{Conn: pipe}
	w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}, sequence: 17}
	defer w.channel.Close()
	w.started.Store(true)
	w.epoch.Store(23)
	var checks atomic.Int32
	w.channel.authority.checkInput = func() error { checks.Add(1); return ErrAuthority }
	w.channel.authority.checkPresent = func() error { checks.Add(1); return ErrAuthority }
	if err := w.sendBoundaries([]sourcePacket{boundaryBatchPacket(999)}); err != nil {
		t.Fatal("discarded advisory traffic requested new authority", err)
	}
	bad := sourcePacket{kind: sourceAction, reply: 23, action: control.InputAction{Kind: control.InputKeyDown, Code: 0x1D}}
	if err := w.sendBoundaries([]sourcePacket{bad}); !errors.Is(err, ErrProtocol) {
		t.Fatal("active advisory suppression hid an invalid input batch", err)
	}
	if checks.Load() != 0 || stream.writes.Load() != 0 || w.sequence != 17 || w.broken {
		t.Fatal("discarded advisory traffic occupied authority or wire capacity")
	}
}
