//go:build windows

package inputworker

import (
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func boundaryBatchPacket(x int32) sourcePacket {
	return sourcePacket{kind: sourceBoundary, x: x, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}}
}

func TestSourceBoundaryBatchPreservesEverySampleAndAuthenticatesOneWrite(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}, sequence: 3}
	defer w.channel.Close()
	w.started.Store(true)
	var checked atomic.Int32
	w.channel.authority.checkInput = func() error { checked.Add(1); return nil }
	w.channel.authority.checkPresent = func() error { return nil }
	var batch [sourceBoundaryBatchLimit]sourcePacket
	// Deliberate edge/retreat/edge changes must survive in their exact order.
	for i := range batch {
		batch[i] = boundaryBatchPacket(int32(i % 3))
	}
	done := make(chan error, 1)
	go func() { done <- w.sendBoundaries(batch[:]) }()
	for i, want := range batch {
		p, err := readSourcePacket(server)
		if err != nil || p.kind != sourceBoundary || p.x != want.x || p.action != want.action || p.binding != w.binding || p.sequence != uint64(i+4) {
			t.Fatal("batch changed packet order, contents or identity", i, p, err)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if checked.Load() != 1 || w.sequence != 3+sourceBoundaryBatchLimit {
		t.Fatal("batch repeated checks or lost sequence")
	}
}

func TestBoundaryPreflightDoesNotBlockHeartbeatReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
		defer w.channel.Close()
		w.started.Store(true)
		entered, release := make(chan struct{}), make(chan struct{})
		w.channel.authority.checkInput = func() error { close(entered); <-release; return nil }
		w.channel.authority.checkPresent = func() error { return nil }
		batchDone, replyDone := make(chan error, 1), make(chan error, 1)
		go func() { batchDone <- w.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)}) }()
		<-entered
		go func() { replyDone <- w.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 1}, nil) }()
		_ = server.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		first, firstErr := readSourcePacket(server)
		close(release)
		_ = server.SetReadDeadline(time.Now().Add(time.Second))
		second, secondErr := readSourcePacket(server)
		if firstErr != nil {
			_, _ = readSourcePacket(server)
		}
		batchErr, replyErr := <-batchDone, <-replyDone
		if firstErr != nil || secondErr != nil || batchErr != nil || replyErr != nil || first.op != sourceHeartbeat || first.sequence != 1 || second.kind != sourceBoundary || second.sequence != 2 {
			t.Fatal("advisory preflight held heartbeat reply serialization", first, second, firstErr, secondErr, batchErr, replyErr)
		}
	})
}

func TestSourceBoundaryBatchCannotTransmitInputOrRejectedAuthority(t *testing.T) {
	for _, reject := range []bool{false, true} {
		server, conn := net.Pipe()
		w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
		w.started.Store(true)
		batch := []sourcePacket{boundaryBatchPacket(1), {kind: sourceAction, action: control.InputAction{Kind: control.InputKeyDown, Code: 65}}}
		if reject {
			w.channel.authority.checkInput = func() error { return ErrAuthority }
			batch = batch[:1]
		}
		if err := w.sendBoundaries(batch); err == nil || w.sequence != 0 {
			t.Fatal("invalid batch sent or consumed sequence", err)
		}
		_ = w.channel.Close()
		_ = server.Close()
	}
}

func TestBoundaryBatchDrainIsBoundedAndYieldsToCommands(t *testing.T) {
	events := make(chan sourcePacket, 64)
	commands := make(chan sourcePacket, 1)
	for i := range 64 {
		events <- boundaryBatchPacket(int32(i + 1))
	}
	var batch [sourceBoundaryBatchLimit]sourcePacket
	if n := collectSourceBoundaries(boundaryBatchPacket(0), events, commands, &batch); n != sourceBoundaryBatchLimit || len(events) != 49 {
		t.Fatal("unbounded or missing batch", n, len(events))
	}
	commands <- sourcePacket{kind: sourceCommand, op: sourceActivate}
	if n := collectSourceBoundaries(boundaryBatchPacket(0), events, commands, &batch); n != 1 || len(events) != 49 || len(commands) != 1 {
		t.Fatal("batch overtook or consumed pending activation", n)
	}
}
