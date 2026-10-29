//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/peer"
)

func TestSourceLateAdmissionRejectsIntentAndRequiresFreshQuery(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
			defer c.channel.Close()
			c.lastReplyRequestAt = time.Now()
			previous := c.lastReplyRequestAt
			go func() {
				first, err := readSourcePacket(server)
				if err != nil {
					return
				}
				time.Sleep(1200 * time.Millisecond)
				reply := sourcePacket{kind: sourceReply, op: first.op, reply: first.sequence, decision: peer.BoundaryAdmissionAllowed}
				if rejected {
					reply.status, reply.decision = 1, 0
				}
				c.replies <- reply
				second, err := readSourcePacket(server)
				if err == nil {
					// A fresh blocked result must override the old permission.
					c.replies <- sourcePacket{kind: sourceReply, op: second.op, reply: second.sequence, decision: peer.BoundaryAdmissionBlockedPressedInput}
				}
			}()
			p, err := c.call(context.Background(), sourceAdmit)
			decision, err := sourceAdmissionResult(p, err)
			if decision != 0 || !errors.Is(err, peer.ErrBoundaryInspectionUnavailable) || time.Since(previous) != time.Second {
				t.Fatal("late inspection did not reject exactly the old intent", decision, err)
			}
			if c.channel.authority.closed || c.epoch.Load() != 0 || c.retiring.Load() || c.pending.sequence != 1 {
				t.Fatal("idle inspection timeout retired or activated the owner")
			}
			if attempted, err := c.pollHeartbeat(context.Background()); attempted || err != nil {
				t.Fatal("maintenance wrote across an unresolved reply", attempted, err)
			}
			if _, err := c.call(context.Background(), sourceActivate); err != peer.ErrBoundaryInspectionUnavailable || c.sequence != 1 || c.epoch.Load() != 0 {
				t.Fatal("pending inspection permitted another wire command", err)
			}
			time.Sleep(200 * time.Millisecond)
			synctest.Wait()
			pending, err := c.drainAdmission(context.Background())
			if pending || err != nil || !c.lastReplyRequestAt.Equal(previous) || c.epoch.Load() != 0 {
				t.Fatal("drain reused a late permission or renewed liveness", pending, err)
			}
			p, err = c.call(context.Background(), sourceAdmit)
			decision, err = sourceAdmissionResult(p, err)
			if err != nil || decision != peer.BoundaryAdmissionBlockedPressedInput || c.sequence != 2 || c.pending.sequence != 0 {
				t.Fatal("next intent did not use a fresh authenticated query", decision, err)
			}
		})
	}
}

func TestSourceAdmissionDrainKeepsFailureBoundaries(t *testing.T) {
	for _, reason := range []string{"missing", "foreign", "invalid", "closed", "revoked", "revoked_pending"} {
		t.Run(reason, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				server, conn := net.Pipe()
				defer server.Close()
				c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
				defer c.channel.Close()
				go func() { _, _ = readSourcePacket(server) }()
				if _, err := c.call(context.Background(), sourceAdmit); err != peer.ErrBoundaryInspectionUnavailable {
					t.Fatal(err)
				}
				limit := c.admissionUntil
				switch reason {
				case "missing":
					time.Sleep(RecoveryTimeout)
				case "foreign":
					c.replies <- sourcePacket{kind: sourceReply, op: sourceAdmit, reply: 99, decision: peer.BoundaryAdmissionAllowed}
				case "invalid":
					c.replies <- sourcePacket{kind: sourceReply, op: sourceAdmit, reply: 1}
				case "closed":
					close(c.done)
				case "revoked":
					c.replies <- sourcePacket{kind: sourceReply, op: sourceAdmit, reply: 1, decision: peer.BoundaryAdmissionAllowed}
					c.channel.authority.checkInput = func() error { return ErrAuthority }
				case "revoked_pending":
					c.channel.authority.checkInput = func() error { return ErrAuthority }
				}
				if _, err := c.call(context.Background(), sourceActivate); err == nil || err == peer.ErrBoundaryInspectionUnavailable || !c.channel.authority.closed || c.sequence != 1 || c.epoch.Load() != 0 {
					t.Fatal("unresolved/invalid inspection permitted capture", err)
				}
				if reason == "missing" && !time.Now().Equal(limit) {
					t.Fatal("drain deadline extended")
				}
			})
		})
	}
}

func TestSourceAdmissionTimeoutCannotKeepActiveOrCursorOwner(t *testing.T) {
	for _, cursor := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{}), cursorPrepared: cursor}
			defer c.channel.Close()
			if !cursor {
				c.epoch.Store(42)
			}
			go func() { _, _ = readSourcePacket(server) }()
			_, err := c.call(context.Background(), sourceAdmit)
			if err == nil || err == peer.ErrBoundaryInspectionUnavailable || !c.channel.authority.closed || c.epoch.Load() != 0 || c.pending.sequence != 0 {
				t.Fatal("active input or cursor state was retained after missing reply", err)
			}
		})
	}
}

func TestSourceAdmissionDrainCannotExtendStandbyIdleDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		defer c.channel.Close()
		c.lastReplyRequestAt = time.Now().Add(-sourceStandbyTimeout + 1500*time.Millisecond)
		deadline := c.lastReplyRequestAt.Add(sourceStandbyTimeout)
		go func() { _, _ = readSourcePacket(server) }()
		if _, err := c.call(context.Background(), sourceAdmit); err != peer.ErrBoundaryInspectionUnavailable || !c.admissionUntil.Equal(deadline) {
			t.Fatal("drain extended old owner idle deadline", err, c.admissionUntil)
		}
		time.Sleep(500 * time.Millisecond)
		if attempted, err := c.pollHeartbeat(context.Background()); attempted || err != ErrAuthority || !c.channel.authority.closed || c.sequence != 1 {
			t.Fatal("expired drain was acknowledged or wrote maintenance", attempted, err)
		}
	})
}

func TestSourceStopDrainsLateAdmissionBeforeIndependentRetirement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		defer c.channel.Close()
		go func() {
			p, err := readSourcePacket(server)
			if err != nil {
				return
			}
			time.Sleep(1200 * time.Millisecond)
			c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence, decision: peer.BoundaryAdmissionAllowed}
			stop, err := readSourcePacket(server)
			if err == nil && stop.op == sourceStop && stop.sequence == 2 {
				c.retired.Store(true)
				close(c.done) // Models independently authenticated native retirement.
			}
		}()
		if _, err := c.call(context.Background(), sourceAdmit); err != peer.ErrBoundaryInspectionUnavailable {
			t.Fatal(err)
		}
		if _, err := c.call(context.Background(), sourceStop); err != nil || !c.retired.Load() || c.pending.sequence != 0 || c.sequence != 2 || c.epoch.Load() != 0 {
			t.Fatal("late inspection substituted for stop or blocked cleanup", err)
		}
	})
}
