//go:build windows

package inputworker

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestBoundaryFullProofFollowsSlowPresence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
		defer w.channel.Close()
		w.started.Store(true)
		entered, release := make(chan struct{}), make(chan struct{})
		var revoked atomic.Bool
		w.channel.authority.checkPresent = func() error { close(entered); <-release; return nil }
		w.channel.authority.checkInput = func() error {
			if revoked.Load() {
				return ErrAuthority
			}
			return nil
		}
		done := make(chan error, 1)
		go func() { done <- w.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)}) }()
		<-entered
		revoked.Store(true)
		close(release)
		if err := <-done; !errors.Is(err, ErrAuthority) || w.sequence != 0 {
			t.Fatal("stale full proof survived presence preflight", err, w.sequence)
		}
	})
}

func TestBoundaryPresencePreflightYieldsWriterToHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
		defer w.channel.Close()
		w.started.Store(true)
		entered, release := make(chan struct{}), make(chan struct{})
		var checks atomic.Int32
		w.channel.authority.checkInput = func() error { return nil }
		w.channel.authority.checkPresent = func() error {
			if checks.Add(1) == 1 {
				close(entered)
				<-release
			}
			return nil
		}
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
			t.Fatal("advisory presence held heartbeat writer", firstErr, secondErr, batchErr, replyErr, first, second)
		}
	})
}
