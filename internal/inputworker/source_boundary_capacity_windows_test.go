//go:build windows

package inputworker

import (
	"context"
	"net"
	"testing"
	"testing/synctest"
	"time"
)

func TestBoundaryCapacityExpiryKeepsHeartbeatOwnerAndSequence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		release, finish := occupyAuthorityQueries(t)
		defer finish()
		server, pipe := net.Pipe()
		defer server.Close()
		stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
		owner := clientChannel(stream, authorityAgent)
		defer owner.Close()
		writer := &sourceWriter{ctx: context.Background(), channel: owner, binding: Binding{1}}
		writer.started.Store(true)
		started := time.Now()
		if err := writer.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)}); err != nil || time.Since(started) != time.Second || writer.sequence != 0 || writer.broken || owner.authority.fenced.Load() || stream.writes.Load() != 0 {
			t.Fatal("unsent capacity expiry ended capture or consumed a sequence", err, writer.sequence)
		}
		release()
		done := make(chan error, 1)
		go func() {
			done <- writer.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 7}, nil)
		}()
		_ = server.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		packet, readErr := readSourcePacket(server)
		if err := <-done; err != nil || readErr != nil || packet.sequence != 1 || packet.op != sourceHeartbeat || packet.reply != 7 || stream.writes.Load() != 1 {
			t.Fatal("capacity release could not resume fresh heartbeat or replayed stale observations", err, readErr, packet)
		}
	})
}
