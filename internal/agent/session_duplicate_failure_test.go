package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
)

// Force the local peer read to finish before the source's Interrupt returns.
// Every connection/native cleanup still really runs; no sleeps choose the winner.
type sendFailureConnection struct {
	*framedConnection
	sendErr, closeErr error
	failed, closed    atomic.Bool
	peerCanceled      <-chan struct{}
}

func (c *sendFailureConnection) WriteFrameContext(ctx context.Context, f transport.Frame) error {
	if f.Kind == transport.MessageInput {
		c.failed.Store(true)
		return c.sendErr
	}
	return c.framedConnection.WriteFrameContext(ctx, f)
}
func (c *sendFailureConnection) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	err := c.framedConnection.Close()
	if c.failed.Load() {
		<-c.peerCanceled
	}
	return errors.Join(err, c.closeErr)
}

type cleanupFailureTrigger struct {
	hook    triggerHook
	failure error
}

func (h *cleanupFailureTrigger) RunWithBoundary(ctx context.Context, s *wininput.SourceSession, b wininput.BoundaryObserver, ready func()) error {
	return errors.Join(h.hook.RunWithBoundary(ctx, s, b, ready), h.failure)
}

func TestSessionReadWinsSendFailureWithoutInventingRecoveryFailure(t *testing.T) {
	for _, mode := range []string{"clean", "native-cleanup-failed", "connection-cleanup-failed"} {
		t.Run(mode, func(t *testing.T) {
			left, right := openConnectionPair(t)
			defer right.Close()
			peerCanceled := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(peerCanceled) }) }
			defer release()
			sendErr := errors.New("socket write failed")
			closeErr, nativeErr := error(nil), error(nil)
			if mode == "connection-cleanup-failed" {
				closeErr = errors.New("connection close failed")
			}
			if mode == "native-cleanup-failed" {
				nativeErr = errors.New("native owner did not retire")
			}
			conn := &sendFailureConnection{framedConnection: left, sendErr: sendErr, closeErr: closeErr, peerCanceled: peerCanceled}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cfg := DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10})
			cfg.TerminationObserver = func(v SessionTermination) {
				if v.Trigger && v.Component == "peer-session" {
					release()
				}
			}
			otherDone := make(chan struct{})
			go func() {
				defer close(otherDone)
				_, _ = RunSession(ctx, DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeLeft, ScreenWidth: 100, RearmDistance: 10}), Dependencies{Connection: right, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}})
			}()
			done := make(chan struct {
				r Result
				e error
			}, 1)
			go func() {
				r, e := RunSession(ctx, cfg, Dependencies{Connection: conn, Hook: &cleanupFailureTrigger{hook: triggerHook{returned: make(chan struct{})}, failure: nativeErr}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}})
				done <- struct {
					r Result
					e error
				}{r, e}
			}()
			select {
			case v := <-done:
				if !errors.Is(v.e, sendErr) || !errors.Is(v.e, peer.ErrPeerReadAfterLocalInterrupt) {
					t.Fatal("lost correlated send/read diagnostics", v.e)
				}
				if mode == "clean" {
					if errors.Is(v.e, ErrRecoveryUnverified) || v.r.FinalState != control.StateLocal || v.r.Recovery.NativeOwnersReleased != RecoveryConfirmed || v.r.Recovery.PressedLedgerEmpty != RecoveryConfirmed {
						t.Fatal("clean read-first shutdown cannot reconnect", v.r, v.e)
					}
				} else if !errors.Is(v.e, ErrRecoveryUnverified) || closeErr != nil && !errors.Is(v.e, closeErr) || nativeErr != nil && !errors.Is(v.e, nativeErr) {
					t.Fatal("real cleanup failure lost", v.e)
				}
			case <-ctx.Done():
				release()
				t.Fatal("read-first shutdown stalled", ctx.Err())
			}
			cancel()
			select {
			case <-otherDone:
			case <-time.After(time.Second):
				t.Fatal("remote session leaked")
			}
		})
	}
}
