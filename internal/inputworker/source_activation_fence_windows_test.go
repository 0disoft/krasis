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

func TestSourceActivationQueuedInputCannotCompeteWithProofOrReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		run, cancel := context.WithCancel(context.Background())
		defer cancel()
		w := &sourceWriter{ctx: run, channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
		defer w.channel.Close()
		w.started.Store(true)
		var checks atomic.Int32
		w.channel.authority.checkInput = func() error { checks.Add(1); return nil }
		w.channel.authority.checkPresent = func() error { return nil }
		fence := w.beginActivation(23)
		w.epoch.Store(23)
		session, _ := wininput.NewSourceSession(8, w)
		ready, joined := make(chan struct{}), make(chan error, 1)
		go func() { joined <- session.RunWithReady(run, func() { close(ready) }) }()
		<-ready
		if err := session.Activate(); err != nil {
			t.Fatal(err)
		}
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
		if !session.Offer(move) {
			t.Fatal("first movement rejected")
		}
		synctest.Wait() // Sender has claimed the first move and waits on the fence.
		for range 999 {
			if !session.Offer(move) {
				t.Fatal("queued movement rejected")
			}
		}
		down := control.InputAction{Kind: control.InputKeyDown, Code: 30}
		up := control.InputAction{Kind: control.InputKeyUp, Code: 30}
		if !session.Offer(down) || !session.Offer(up) {
			t.Fatal("queued key transition rejected")
		}
		synctest.Wait()
		if checks.Load() != 0 || w.sequence != 0 {
			t.Fatal("input started full authority or wire work before activation completion")
		}
		request := sourcePacket{kind: sourceCommand, op: sourceActivate, sequence: 23, binding: w.binding}
		completed := make(chan error, 1)
		go func() {
			err := w.completeSourceCommand(request, sourcePacket{kind: sourceReply, op: sourceActivate, reply: 23}, nil, fence)
			completed <- err
		}()
		ack, err := readSourcePacket(server)
		if err != nil || ack.kind != sourceReply || ack.reply != 23 || ack.sequence != 1 {
			t.Fatal("input preceded the real activation ACK", ack, err)
		}
		if err := <-completed; err != nil {
			t.Fatal(err)
		}
		var delta int32
		for sequence, want := range []control.InputAction{move, {Kind: control.InputPointerMove, DeltaX: 999}, down, up} {
			p, err := readSourcePacket(server)
			if err != nil || p.kind != sourceAction || p.reply != 23 || p.sequence != uint64(sequence+2) || p.action != want {
				t.Fatal("activation wait lost/coalesced discrete input or changed generation/order", p, want, err)
			}
			if p.action.Kind == control.InputPointerMove {
				delta += p.action.DeltaX
			}
		}
		if delta != 1000 || checks.Load() != 4 {
			t.Fatal("movement or fresh per-frame authority lost", delta, checks.Load())
		}
		session.Deactivate()
		cancel()
		<-joined
	})
}

func TestSourceActivationNegativeACKOrFailedWriteNeverReleasesInput(t *testing.T) {
	for _, failWrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "negative-ack", true: "failed-write"}[failWrite], func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			w.channel.authority.checkPresent = func() error { return nil }
			fence := w.beginActivation(23)
			w.epoch.Store(23) // A negative ACK must reject even if this epoch remains current.
			request := sourcePacket{kind: sourceCommand, op: sourceActivate, sequence: 23, binding: w.binding}
			response := sourcePacket{kind: sourceReply, op: sourceActivate, reply: 23, status: 1}
			operation := errors.New("synthetic activation rejection")
			received := make(chan sourcePacket, 1)
			if failWrite {
				server.Close()
				operation, response.status = nil, 0
			} else {
				go func() {
					p, _ := readSourcePacket(server)
					received <- p
				}()
			}
			err := w.completeSourceCommand(request, response, operation, fence)
			if (err != nil) != failWrite {
				t.Fatal("safe rejection or failed delivery classification changed", err)
			}
			if !failWrite {
				if p := <-received; p.kind != sourceReply || p.status != 1 {
					t.Fatal("negative outcome was changed", p)
				}
			}
			if err := w.awaitActivation(context.Background(), func() bool { return true }); !errors.Is(err, control.ErrSourceActionDiscarded) {
				t.Fatal("failed activation released queued input", err)
			}
		})
	}
}

func TestSourceActivationFenceRejectsFailedOrObsoleteInput(t *testing.T) {
	for _, reason := range []string{"rejected", "epoch", "replacement", "stale", "cancel-operation", "cancel-owner"} {
		t.Run(reason, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				owner, stop := context.WithCancel(context.Background())
				defer stop()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				w := &sourceWriter{ctx: owner}
				fence := w.beginActivation(23)
				w.epoch.Store(23)
				var stale atomic.Bool
				done := make(chan error, 1)
				go func() { done <- w.awaitActivation(ctx, func() bool { return !stale.Load() }) }()
				synctest.Wait()
				switch reason {
				case "epoch":
					w.epoch.Store(24)
				case "replacement":
					w.beginActivation(24)
				case "stale":
					stale.Store(true)
				case "cancel-operation":
					cancel()
				case "cancel-owner":
					stop()
				}
				want := control.ErrSourceActionDiscarded
				if reason == "cancel-operation" || reason == "cancel-owner" {
					want = context.Canceled // No completion is required for cancellation.
				} else {
					finishSourceActivation(fence, reason != "rejected")
				}
				if err := <-done; !errors.Is(err, want) {
					t.Fatal("failed/stale activation released input or cancellation was blocked", err)
				}
			})
		})
	}
}
