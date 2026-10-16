//go:build windows

package inputworker

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestBoundarySenderBlockedAuthorizationCannotDelayCommandReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
		defer w.channel.Close()
		w.started.Store(true)
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		defer once.Do(func() { close(release) })
		var checks atomic.Int32
		w.channel.authority.checkInput = func() error {
			if checks.Add(1) == 1 {
				close(entered)
				<-release
			}
			return nil
		}
		w.channel.authority.checkPresent = func() error { return nil }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		events := make(chan sourcePacket, 64)
		events <- boundaryBatchPacket(1)
		pumped := make(chan error, 1)
		go func() { pumped <- pumpSourceBoundaries(ctx, events, w.sendBoundaries) }()
		<-entered
		commands := make(chan sourcePacket, 1)
		commands <- sourcePacket{kind: sourceCommand, op: sourceActivate, sequence: 1}
		work, err := nextSourceWork(ctx, commands, nil, nil)
		if err != nil || work.packet.op != sourceActivate {
			t.Fatal("native command could not run during advisory authorization", err)
		}
		replied := make(chan error, 1)
		go func() {
			replied <- w.send(sourcePacket{kind: sourceReply, op: work.packet.op, reply: work.packet.sequence}, nil)
		}()
		_ = server.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		ack, ackErr := readSourcePacket(server)
		once.Do(func() { close(release) })
		_ = server.SetReadDeadline(time.Now().Add(time.Second))
		observation, observationErr := readSourcePacket(server)
		if ackErr != nil {
			_, _ = readSourcePacket(server) // Drain both writers before reporting a failed assertion.
		}
		cancel()
		replyErr, pumpErr := <-replied, <-pumped
		if ackErr != nil || observationErr != nil || replyErr != nil || pumpErr != context.Canceled || ack.op != sourceActivate || ack.sequence != 1 || observation.kind != sourceBoundary || observation.x != 1 || observation.sequence != 2 || checks.Load() != 2 {
			t.Fatal("advisory query delayed command ACK or lost/reordered packets", ack, observation, ackErr, observationErr, replyErr, pumpErr)
		}
	})
}

func TestBoundarySenderPreservesBoundedBatchOrderOnQueueClosure(t *testing.T) {
	events := make(chan sourcePacket, 64)
	for n := 0; n < 33; n++ {
		events <- boundaryBatchPacket(int32(n))
	}
	close(events)
	count, batches := 0, 0
	err := pumpSourceBoundaries(context.Background(), events, func(batch []sourcePacket) error {
		batches++
		if len(batch) == 0 || len(batch) > sourceBoundaryBatchLimit {
			t.Fatal("unbounded or empty batch", len(batch))
		}
		for _, p := range batch {
			if p.kind != sourceBoundary || p.x != int32(count) {
				t.Fatal("boundary sample was lost or reordered", p, count)
			}
			count++
		}
		return nil
	})
	if err != nil || count != 33 || batches != 3 {
		t.Fatal("queue closure lost samples", err, count, batches)
	}
}

func TestBoundarySenderCancellationWaitsForInFlightWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan sourcePacket, 64)
	events <- boundaryBatchPacket(1)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	done := make(chan error, 1)
	go func() {
		done <- pumpSourceBoundaries(ctx, events, func([]sourcePacket) error { close(entered); <-release; return nil })
	}()
	<-entered
	events <- boundaryBatchPacket(2)
	cancel()
	select {
	case err := <-done:
		t.Fatal("sender certified retirement before its write returned", err)
	default:
	}
	once.Do(func() { close(release) })
	if err := <-done; err != context.Canceled || len(events) != 1 {
		t.Fatal("canceled sender wrote another sample or lost cancellation", err, len(events))
	}
}
