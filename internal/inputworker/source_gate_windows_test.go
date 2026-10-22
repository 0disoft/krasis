//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
)

type gateObservedStream struct {
	net.Conn
	entered chan struct{}
	once    sync.Once
}

func (s *gateObservedStream) Write(p []byte) (int, error) {
	s.once.Do(func() { close(s.entered) })
	return s.Conn.Write(p)
}

func TestHeartbeatGateFailureRetainsObservedHolder(t *testing.T) {
	for _, origin := range []string{"boundary", "action", "reply", "return", "retirement", "presence"} {
		t.Run(origin, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			stream := &gateObservedStream{Conn: conn, entered: make(chan struct{})}
			w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			w.started.Store(true)
			entered := stream.entered
			release := make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			if origin == "presence" {
				entered = make(chan struct{})
				w.channel.authority.checkPresent = func() error { close(entered); <-release; return nil }
			}
			owner := make(chan error, 1)
			go func() {
				p := sourcePacket{kind: sourceReturn, reply: 1}
				switch origin {
				case "boundary":
					owner <- w.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)})
					return
				case "action":
					p.kind = sourceAction
					p.action = control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
				case "reply":
					p.kind, p.op = sourceReply, sourceHeartbeat
				case "retirement":
					p.kind, p.op = sourceReply, sourceStop
				}
				owner <- w.write(p, nil, origin == "retirement")
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("owner did not enter observed stage")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			failure := w.writePacketContext(ctx, sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 2}, nil, false, false)
			cancel()
			var gate *sourceGateFailure
			want := "source-gate-" + origin + "-write"
			if origin == "presence" {
				want = "source-gate-return-presence"
			}
			if !errors.Is(failure, context.DeadlineExceeded) || !errors.As(failure, &gate) || gate.operation != want {
				t.Fatal("gate failure lost holder or deadline", failure, gate, want)
			}
			releaseOnce.Do(func() { close(release) })
			_ = server.SetReadDeadline(time.Now().Add(time.Second))
			packet, err := readSourcePacket(server)
			ownerErr := <-owner
			if err != nil || ownerErr != nil || packet.sequence != 1 || w.sequence != 1 || w.broken || w.gateOwner.Load() != 0 {
				t.Fatal("diagnostic waiter changed owner write or leaked holder", packet, err, ownerErr, w.gateOwner.Load())
			}
			final := sourceCaptureFinal(sourceCommandFailed(sourceHeartbeat, failure), context.Canceled, context.Canceled, false)
			final = sourceRetirementDelivery(final, &sourceIOFailure{stage: "write", origin: sourceRetired, cause: net.ErrClosed})
			var wire bytes.Buffer
			if err := writeReport(&wire, Report{}, final); err != nil {
				t.Fatal(err)
			}
			_, received := readReport(&wire)
			code, _ := reportFailureDetails(received)
			if reportOperations[code] != want || errors.Is(final, ErrRecovery) {
				t.Fatal("gate observation lost on final report", received)
			}
			unsafe := sourceCaptureFinal(failure, inputdesktop.ErrCleanup, context.Canceled, false)
			code, _ = reportFailureDetails(unsafe)
			if !errors.Is(unsafe, ErrRecovery) || reportOperations[code] == want {
				t.Fatal("gate observation hid native cleanup uncertainty")
			}
		})
	}
}

func TestSourceGateClosedLabelsAndUnobservedOwner(t *testing.T) {
	for _, operation := range logindiag.SourceGateOperations() {
		if !logindiag.ValidRuntimeOperation(operation) {
			t.Fatal("fixed gate operation cannot be exported", operation)
		}
	}
	for _, operation := range []string{"source-gate-key-write", "source-gate-action-private", "source-gate-action-write-private"} {
		if logindiag.ValidRuntimeOperation(operation) {
			t.Fatal("free-form label admitted", operation)
		}
	}
	if sourceGateOperation(0) != "source-gate-unobserved" || len(reportOperations) > 256 {
		t.Fatal("unknown owner or operation byte overflow")
	}
}
