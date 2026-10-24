//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceWriterNativeChecksRespectSendActionBudget(t *testing.T) {
	for _, presence := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, pipe := net.Pipe()
			defer server.Close()
			stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
			channel := clientChannel(stream, authorityAgent)
			w := &sourceWriter{channel: channel, binding: Binding{1}}
			w.started.Store(true)
			w.epoch.Store(42)
			release := make(chan struct{})
			var closes atomic.Int32
			channel.authority.closeHandles = func() error { closes.Add(1); return nil }
			native := func() error {
				<-release
				if closes.Load() != 0 || stream.closes.Load() != 0 {
					t.Error("source writer lost native query handles before return")
				}
				return nil
			}
			if presence {
				channel.authority.checkPresent = native
			} else {
				channel.authority.checkInput = native
			}
			timer := time.AfterFunc(time.Second, func() { close(release) })
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			started := time.Now()
			err := w.SendAction(ctx, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, nil)
			if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 50*time.Millisecond {
				t.Fatalf("writer native query ignored caller deadline: presence=%t elapsed=%s err=%v", presence, time.Since(started), err)
			}
			if stream.writes.Load() != 0 || w.sequence != 0 || closes.Load() != 0 || stream.closes.Load() != 0 || channel.Close() != ErrAuthority {
				t.Fatal("expired writer query submitted bytes or discarded native ownership")
			}
			if timer.Stop() {
				close(release)
			}
			synctest.Wait()
			if channel.Close() != nil || closes.Load() != 1 || stream.closes.Load() != 1 {
				t.Fatal("late writer query failed to retire exactly once")
			}
		})
	}
}

func TestSourceWriterGateAndWireShareCallerBudget(t *testing.T) {
	for _, waitGate := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, pipe := net.Pipe()
			defer server.Close()
			w := &sourceWriter{channel: clientChannel(pipe, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			w.started.Store(true)
			w.epoch.Store(42)
			var timer *time.Timer
			if waitGate {
				w.mu.Lock()
				timer = time.AfterFunc(time.Second, w.mu.Unlock)
			} else {
				w.channel.authority.checkInput = func() error { time.Sleep(40 * time.Millisecond); return nil }
				w.channel.authority.checkPresent = func() error { return nil }
			}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			started := time.Now()
			err := w.SendAction(ctx, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, nil)
			if timer != nil && timer.Stop() {
				w.mu.Unlock()
			}
			if err == nil || time.Since(started) != 50*time.Millisecond {
				t.Fatalf("writer renewed its budget after preflight: gate=%t elapsed=%s err=%v", waitGate, time.Since(started), err)
			}
			if waitGate && (w.sequence != 0 || w.broken || w.channel.authority.closed) {
				t.Fatal("unsent waiter changed sequence or retired an available owner")
			}
			if !waitGate && (w.sequence != 1 || !w.broken) {
				t.Fatal("attempted write failure did not poison the stream")
			}
		})
	}
}

func TestSourceWriterOwnerCancellationDoesNotBlockJoinedRetirement(t *testing.T) {
	server, pipe := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	w := &sourceWriter{ctx: ctx, channel: clientChannel(pipe, authorityAgent), binding: Binding{1}}
	defer w.channel.Close()
	w.started.Store(true)
	w.epoch.Store(42)
	cancel()
	if err := w.SendAction(context.Background(), control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, nil); !errors.Is(err, context.Canceled) || w.sequence != 0 {
		t.Fatal("a live caller bypassed its canceled source owner", err)
	}
	received := make(chan sourcePacket, 1)
	go func() { p, _ := readSourcePacket(server); received <- p }()
	if err := w.write(sourcePacket{kind: sourceRetired}, nil, true); err != nil {
		t.Fatal("joined retirement inherited the canceled capture context", err)
	}
	if p := <-received; p.kind != sourceRetired || p.sequence != 1 || p.status != 0 {
		t.Fatal("cancellation changed the terminal proof", p)
	}
}

func TestSourceWriterInheritedOperationLifetime(t *testing.T) {
	for _, action := range []string{"cancel_owner", "owner_deadline", "cancel_operation"} {
		t.Run(action, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				owner, cancelOwner := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancelOwner()
				w := &sourceWriter{ctx: owner}
				op, cancel := w.operation(nil, false)
				defer cancel()
				ownerDeadline, _ := owner.Deadline()
				if deadline, ok := op.Deadline(); !ok || !deadline.Equal(ownerDeadline) {
					t.Fatal("operation did not inherit the earlier owner deadline")
				}
				want := context.Canceled
				switch action {
				case "cancel_owner":
					cancelOwner()
				case "owner_deadline":
					time.Sleep(50 * time.Millisecond)
					want = context.DeadlineExceeded
				case "cancel_operation":
					cancel()
					if owner.Err() != nil {
						t.Fatal("canceling one operation canceled its source owner")
					}
				}
				synctest.Wait()
				if !errors.Is(op.Err(), want) {
					t.Fatalf("operation error = %v, want %v", op.Err(), want)
				}
			})
		})
	}
}

func TestSourceBoundaryBatchReauthorizesAfterGateContention(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		w := &sourceWriter{channel: clientChannel(pipe, authorityAgent), binding: Binding{1}}
		defer w.channel.Close()
		w.started.Store(true)
		var checks atomic.Int32
		checked := make(chan struct{})
		w.channel.authority.checkInput = func() error {
			if checks.Add(1) == 1 {
				close(checked)
				return nil
			}
			return ErrAuthority
		}
		w.mu.Lock()
		done := make(chan error, 1)
		go func() { done <- w.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)}) }()
		<-checked
		synctest.Wait()
		w.mu.Unlock()
		if err := <-done; !errors.Is(err, ErrAuthority) || checks.Load() != 2 || w.sequence != 0 {
			t.Fatal("advisory batch reused full authority after a writer wait", err, checks.Load())
		}
	})
}
