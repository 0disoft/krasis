//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourcePointerProofExpiryKeepsFreshCommandsAndNeverReplays(t *testing.T) {
	for _, presence := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			peer, conn := net.Pipe()
			defer peer.Close()
			wire := &sourceGenerationWire{Conn: conn}
			channel := clientChannel(wire, authorityAgent)
			defer channel.Close()
			w := &sourceWriter{channel: channel, binding: Binding{1}}
			w.started.Store(true)
			w.epoch.Store(5)
			release := make(chan struct{})
			var proofs, checks atomic.Int32
			channel.authority.checkInput = func() error {
				if proofs.Add(1) == 1 && !presence {
					<-release
				}
				return nil
			}
			channel.authority.checkPresent = func() error {
				if checks.Add(1) == 1 && presence {
					<-release
				}
				return nil
			}
			move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 17}
			started := time.Now()
			if err := w.SendAction(context.Background(), move, nil); err != nil || time.Since(started) != time.Second {
				close(release)
				t.Fatalf("unsent pointer proof poisoned capture: presence=%t err=%v elapsed=%s", presence, err, time.Since(started))
			}
			channel.queryMu.Lock()
			retained := channel.maintenanceQuery != nil && !channel.closing
			channel.queryMu.Unlock()
			if !retained || channel.authority.fenced.Load() || wire.wire.Len() != 0 || w.sequence != 0 {
				t.Fatal("expired pointer escaped or lost retained query ownership")
			}
			if err := w.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 7}, nil); err != nil {
				t.Fatal("pending pointer proof blocked heartbeat", err)
			}
			key := control.InputAction{Kind: control.InputKeyDown, Code: 1}
			if err := w.SendAction(context.Background(), key, nil); err != nil {
				t.Fatal("pending pointer proof blocked independently authorized key", err)
			}
			if w.sequence != 2 || proofs.Load() != 2 {
				t.Fatal("pending proof was reused or spawned replacements", w.sequence, proofs.Load())
			}
			close(release)
			synctest.Wait()
			if wire.wire.Len() != 2*sourcePacketSize || channel.authority.fenced.Load() {
				t.Fatal("late query replayed stale movement or fenced fresh work")
			}
			if err := w.SendAction(context.Background(), move, nil); err != nil || proofs.Load() != 3 {
				t.Fatal("new movement lacked independent full proof", err, proofs.Load())
			}
			reader := bytes.NewReader(wire.wire.Bytes())
			for i, kind := range []byte{sourceReply, sourceAction, sourceAction} {
				p, err := readSourcePacket(reader)
				if err != nil || p.sequence != uint64(i+1) || p.kind != kind || i == 1 && p.action != key || i == 2 && p.action != move {
					t.Fatal("pointer expiry changed input ordering or sequence", i, p, err)
				}
			}
		})
	}
}

func TestSourcePointerRetainedProofCannotBeWaitedForever(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		peer, conn := net.Pipe()
		defer peer.Close()
		wire := &sourceGenerationWire{Conn: conn}
		channel := clientChannel(wire, authorityAgent)
		defer channel.Close()
		w := &sourceWriter{channel: channel, binding: Binding{1}}
		w.started.Store(true)
		w.epoch.Store(5)
		release := make(chan struct{})
		var calls atomic.Int32
		channel.authority.checkInput = func() error { calls.Add(1); <-release; return nil }
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
		if err := w.SendAction(context.Background(), move, nil); err != nil {
			t.Error("first unsent sample was not discarded", err)
		}
		err := w.SendAction(context.Background(), move, nil)
		if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 || wire.wire.Len() != 0 || w.sequence != 0 {
			t.Error("same stuck native query could stall capture indefinitely or replay input", err, calls.Load())
		}
		close(release)
		synctest.Wait()
	})
}

func TestSourcePointerProofDoesNotHideDenialOrAttemptedWrite(t *testing.T) {
	for _, failure := range []string{"denied", "zero-write", "partial-write"} {
		t.Run(failure, func(t *testing.T) {
			peer, conn := net.Pipe()
			defer peer.Close()
			stream := &failingSourceStream{Conn: conn, cause: os.ErrDeadlineExceeded, partial: failure == "partial-write"}
			w := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			w.started.Store(true)
			w.epoch.Store(5)
			if failure == "denied" {
				w.channel.authority.checkInput = func() error { return ErrAuthority }
			}
			err := w.SendAction(context.Background(), control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, nil)
			if err == nil || !errors.Is(err, ErrAuthority) {
				t.Fatal("real failure became a discarded sample", err)
			}
			if failure == "denied" {
				if stream.writes != 0 || w.sequence != 0 {
					t.Fatal("rejected authority reached wire")
				}
			} else if stream.writes != 1 || w.sequence != 1 || !w.broken {
				t.Fatal("uncertain write was normalized or replayed", stream.writes, w.sequence, w.broken)
			}
		})
	}
}

func TestSourceDiscreteProofExpiryStillFences(t *testing.T) {
	for _, action := range []control.InputAction{
		{Kind: control.InputKeyDown, Code: 1}, {Kind: control.InputKeyUp, Code: 1},
		{Kind: control.InputButtonDown, Code: 1}, {Kind: control.InputButtonUp, Code: 1},
		{Kind: control.InputWheel, DeltaY: 1},
	} {
		synctest.Test(t, func(t *testing.T) {
			peer, conn := net.Pipe()
			defer peer.Close()
			wire := &sourceGenerationWire{Conn: conn}
			channel := clientChannel(wire, authorityAgent)
			defer channel.Close()
			w := &sourceWriter{channel: channel, binding: Binding{1}}
			w.started.Store(true)
			w.epoch.Store(5)
			release := make(chan struct{})
			channel.authority.checkInput = func() error { <-release; return nil }
			err := w.SendAction(context.Background(), action, nil)
			if !errors.Is(err, context.DeadlineExceeded) || !channel.authority.fenced.Load() || wire.wire.Len() != 0 || w.sequence != 0 {
				t.Error("discrete input was silently discarded or escaped failed proof", action.Kind, err)
			}
			close(release)
			synctest.Wait()
		})
	}
}
