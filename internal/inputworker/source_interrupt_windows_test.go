//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type sourceInterruptConn struct {
	net.Conn
	once    sync.Once
	writing chan struct{}
}

func TestSourceInterruptStillAbortsAttemptedInputWithoutReplaying(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	stream := &sourceInterruptConn{Conn: conn, writing: make(chan struct{})}
	w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
	defer w.channel.Close()
	w.started.Store(true)
	w.epoch.Store(42)
	done := make(chan error, 1)
	go func() {
		done <- w.SendAction(context.Background(), control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, nil)
	}()
	select {
	case <-stream.writing:
	case err := <-done:
		t.Fatal("input never reached the pipe", err)
	case <-time.After(time.Second):
		t.Fatal("input did not start")
	}
	if err := w.Interrupt(context.Canceled); err != nil {
		t.Fatal(err)
	}
	if err := awaitClient(t, done); !errors.Is(err, os.ErrDeadlineExceeded) || !w.broken || w.sequence != 1 {
		t.Fatal("attempted input lost its uncertain-outcome fence", err, w.broken, w.sequence)
	}
	if err := w.write(sourcePacket{kind: sourceRetired}, nil, true); err == nil || w.sequence != 1 {
		t.Fatal("uncertain attempted bytes became a clean retirement")
	}
}

func TestSourceInterruptBeforeInputPreservesSequenceAndControlTransport(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	_ = server.SetDeadline(time.Now().Add(time.Second))
	stream := &sourceInterruptConn{Conn: conn, writing: make(chan struct{})}
	w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
	defer w.channel.Close()
	w.started.Store(true)
	w.epoch.Store(42)
	if err := w.Interrupt(context.Canceled); err != nil {
		t.Fatal(err)
	}
	if err := w.SendAction(context.Background(), control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, nil); !errors.Is(err, context.Canceled) || w.sequence != 0 || w.broken {
		t.Fatal("stopped input reached the stream or poisoned its control lane", err, w.sequence, w.broken)
	}
	select {
	case <-stream.writing:
		t.Fatal("stopped input attempted a write")
	default:
	}
	done := make(chan error, 1)
	go func() { done <- w.write(sourcePacket{kind: sourceRetired}, nil, true) }()
	p, err := readSourcePacket(server)
	if result := awaitClient(t, done); err != nil || result != nil || p.kind != sourceRetired || p.sequence != 1 {
		t.Fatal("unsent interruption prevented exact retirement", p, err, result)
	}
}

func (c *sourceInterruptConn) Write(data []byte) (int, error) {
	c.once.Do(func() { close(c.writing) })
	return c.Conn.Write(data)
}

func TestSourceInterruptPreservesInFlightControlReplyAndRetirement(t *testing.T) {
	for _, op := range []byte{sourceHeartbeat, sourceDeactivate} {
		t.Run(sourceCommandFailureCode(op), func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			_ = server.SetDeadline(time.Now().Add(time.Second))
			stream := &sourceInterruptConn{Conn: conn, writing: make(chan struct{})}
			w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			done := make(chan error, 1)
			go func() {
				done <- w.sendCompletedReply(sourcePacket{kind: sourceCommand, binding: w.binding, op: op, sequence: 1}, sourcePacket{kind: sourceReply, op: op, reply: 1})
			}()
			select {
			case <-stream.writing:
			case err := <-done:
				t.Fatal("reply never reached the pipe", err)
			case <-time.After(time.Second):
				t.Fatal("reply did not start")
			}
			if err := w.Interrupt(context.Canceled); err != nil {
				t.Fatal(err)
			}
			p, err := readSourcePacket(server)
			result := <-done
			if err != nil || result != nil || p.kind != sourceReply || p.op != op || w.broken {
				t.Fatalf("input interruption severed a control reply: packet=%+v read=%v write=%v broken=%t", p, err, result, w.broken)
			}
			go func() { done <- w.write(sourcePacket{kind: sourceRetired}, nil, true) }()
			p, err = readSourcePacket(server)
			if result := <-done; err != nil || result != nil || p.kind != sourceRetired || p.sequence != 2 {
				t.Fatalf("joined retirement lost its wire: packet=%+v read=%v write=%v", p, err, result)
			}
		})
	}
}
