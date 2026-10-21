//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestBoundaryUnsentPresenceExpiryKeepsHeartbeat(t *testing.T) {
	for _, deny := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: conn}}
			w := &sourceWriter{ctx: context.Background(), channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			w.started.Store(true)
			release := make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			var checks atomic.Int32
			w.channel.authority.checkInput = func() error { return nil }
			w.channel.authority.checkPresent = func() error {
				if checks.Add(1) == 1 {
					<-release
				}
				if deny {
					return ErrAuthority
				}
				return nil
			}
			err := w.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)})
			if err != nil || w.sequence != 0 || stream.writes.Load() != 0 || w.channel.authority.fenced.Load() {
				t.Error("unsent presence query retired capture", err, w.sequence)
			}
			// A late optional query cannot occupy the command presence lane.
			err = w.channel.authorizeSourcePresence(context.Background())
			if (err != nil) != deny {
				t.Error("fresh heartbeat presence unavailable", err)
			}
			once.Do(func() { close(release) })
			synctest.Wait()
			// No late success is reused, and a real rejection is never dropped.
			if !deny {
				w.channel.authority.checkPresent = func() error { return ErrAuthority }
			}
			err = w.sendBoundaries([]sourcePacket{boundaryBatchPacket(2)})
			if !errors.Is(err, ErrAuthority) || w.sequence != 0 || stream.writes.Load() != 0 {
				t.Error("presence rejection was swallowed", err, w.sequence)
			}
		})
	}
}

func TestBoundaryUnsentSlowProofKeepsHeartbeatAndRequiresFreshAuthority(t *testing.T) {
	for _, deny := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: conn}}
			w := &sourceWriter{ctx: context.Background(), channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			w.started.Store(true)
			release := make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			var checks atomic.Int32
			w.channel.authority.checkInput = func() error {
				if checks.Add(1) == 1 {
					<-release
				}
				if deny {
					return ErrAuthority
				}
				return nil
			}
			w.channel.authority.checkPresent = func() error { return nil }
			started := time.Now()
			err := w.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)})
			if err != nil || time.Since(started) != time.Second || w.sequence != 0 || stream.writes.Load() != 0 || w.channel.authority.fenced.Load() {
				t.Error("unsent advisory proof retired the input owner", err, w.sequence, stream.writes.Load())
			}
			// A live pending native query retains its handles and slot. It cannot
			// block an independent fresh presence check used by actual replies.
			if err := w.channel.authorizeSourcePresence(context.Background()); err != nil {
				t.Error("slow advisory proof disabled heartbeat presence", err)
			}
			if stream.closes.Load() != 0 || checks.Load() != 1 {
				t.Error("pending proof was abandoned or replaced")
			}
			replied := make(chan error, 1)
			go func() { replied <- w.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 1}, nil) }()
			_ = server.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			packet, readErr := readSourcePacket(server)
			if replyErr := <-replied; replyErr != nil || readErr != nil || packet.kind != sourceReply || packet.op != sourceHeartbeat || packet.sequence != 1 || w.sequence != 1 {
				t.Error("expired unsent batch prevented actual heartbeat reply", readErr, replyErr, packet)
			}
			once.Do(func() { close(release) })
			synctest.Wait()
			// The late result never authorizes the next batch or input. A new
			// full check must see revocation even when the prior call timed out.
			err = w.channel.authorizeSourceInput(context.Background())
			if (err != nil) != deny || checks.Load() != 2 {
				t.Error("late optional proof cached authority or lost rejection", err, checks.Load())
			}
			if err := w.channel.Close(); err != nil || stream.closes.Load() != 1 {
				t.Error("completed native query leaked channel", err)
			}
		})
	}
}
