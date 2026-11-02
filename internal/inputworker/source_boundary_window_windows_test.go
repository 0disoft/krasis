//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"testing/synctest"
	"time"
)

type boundaryBufferedStream struct {
	net.Conn
	frames chan []byte
}

func (s *boundaryBufferedStream) Write(data []byte) (int, error) {
	s.frames <- bytes.Clone(data)
	return len(data), nil
}

func TestPendingBoundaryBatchCannotHoldHeartbeatGateOrFillPipe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		stream := &boundaryBufferedStream{Conn: conn, frames: make(chan []byte, 4)}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		w := &sourceWriter{ctx: ctx, channel: clientChannel(stream, authorityAgent), binding: Binding{1}, boundary: newSourceBoundaryWindow()}
		defer w.channel.Close()
		w.started.Store(true)
		batch := make([]sourcePacket, sourceBoundaryBatchLimit)
		for i := range batch {
			batch[i] = boundaryBatchPacket(int32(i % 3))
		}
		if err := w.sendBoundaries(batch); err != nil {
			t.Fatal(err)
		}
		wire := <-stream.frames
		reader := bytes.NewReader(wire)
		for i := range batch {
			p, err := readSourcePacket(reader)
			if err != nil || p.x != batch[i].x || p.sequence != uint64(i+1) {
				t.Fatal("sample order lost", p, err)
			}
			if p.reply != 0 && (i != len(batch)-1 || p.reply != p.sequence) {
				t.Fatal("invalid receipt marker", p)
			}
		}
		second := make(chan error, 1)
		go func() { second <- w.sendBoundaries(batch) }()
		time.Sleep(2 * time.Second) // Reproduce an agent stall beyond the old 1s write budget.
		if w.boundary.pending.Load() == nil || len(stream.frames) != 0 {
			t.Fatal("unbounded advisory delivery")
		}
		if err := w.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 1}, nil); err != nil {
			t.Fatal("heartbeat blocked by boundary receipt", err)
		}
		p, err := readSourcePacket(bytes.NewReader(<-stream.frames))
		if err != nil || p.op != sourceHeartbeat || p.sequence != 17 {
			t.Fatal(p, err)
		}
		if err := w.boundary.acknowledge(16); err != nil {
			t.Fatal(err)
		}
		if err := <-second; err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(<-stream.frames)
		for i := range batch {
			p, err = readSourcePacket(reader)
			if err != nil || p.sequence != uint64(i+18) {
				t.Fatal(p, err)
			}
		}
		if p.reply != 33 || w.sequence != 33 || w.broken {
			t.Fatal("receipt changed shared sequence", p, w.sequence)
		}
		cancel()
	})
}

func TestNativeBoundaryWindowReservesPipeSpaceForHeartbeat(t *testing.T) {
	server, agent := nativePair(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &sourceWriter{ctx: ctx, channel: clientChannel(server, authorityAgent), binding: Binding{1}, boundary: newSourceBoundaryWindow()}
	defer w.channel.Close()
	w.started.Store(true)
	batch := make([]sourcePacket, sourceBoundaryBatchLimit)
	for i := range batch {
		batch[i] = boundaryBatchPacket(int32(i % 3))
	}
	// The real 4KiB byte pipe must complete both writes before any reader runs.
	if err := w.sendBoundaries(batch); err != nil {
		t.Fatal("boundary exceeded native pipe reservation", err)
	}
	if err := w.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 1}, nil); err != nil {
		t.Fatal("heartbeat lost reserved pipe capacity", err)
	}
	if err := agent.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < sourceBoundaryBatchLimit+1; i++ {
		p, err := readSourcePacket(agent)
		if err != nil || p.sequence != uint64(i+1) {
			t.Fatal(p, err)
		}
		if i == sourceBoundaryBatchLimit && p.op != sourceHeartbeat {
			t.Fatal("missing heartbeat")
		}
	}
}

func TestBoundaryReceiptUsesPresenceWithoutGrantingInputAuthority(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	channel := clientChannel(conn, authorityService)
	defer channel.Close()
	full := 0
	channel.authority.checkInput = func() error { full++; return ErrAuthority }
	channel.authority.checkPresent = func() error { return nil }
	client := &sourceClient{channel: channel, binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	seen := make(chan sourcePacket, 1)
	go func() {
		p, err := readSourcePacket(server)
		if err != nil {
			close(client.done)
			return
		}
		seen <- p
	}()
	if _, err := client.call(context.Background(), sourceBoundaryAck, 16); err != nil {
		t.Fatal(err)
	}
	if p := <-seen; p.op != sourceBoundaryAck || p.reply != 16 || p.sequence != 1 || full != 0 || client.epoch.Load() != 0 {
		t.Fatal("receipt created input authority", p, full)
	}
}

func TestBoundaryWindowDoesNotHideAttemptedWritesOrRetainUnsentCredit(t *testing.T) {
	for _, unsubmitted := range []bool{false, true} {
		server, conn := net.Pipe()
		var cause error = os.ErrDeadlineExceeded
		if unsubmitted {
			cause = &unsubmittedPipeWrite{cause: cause}
		}
		stream := &failingSourceStream{Conn: conn, cause: cause}
		w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}, boundary: newSourceBoundaryWindow()}
		w.started.Store(true)
		err := w.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)})
		if (err == nil) != unsubmitted || (!unsubmitted && !errors.Is(err, os.ErrDeadlineExceeded)) || w.broken == unsubmitted || stream.writes != 1 || w.boundary.pending.Load() != nil {
			t.Fatal("write policy or reservation lost", unsubmitted, err, w.broken)
		}
		_ = w.channel.Close()
		_ = server.Close()
	}
}
