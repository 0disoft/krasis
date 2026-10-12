//go:build windows

package residentagent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

type secureCaptureTest struct {
	active  atomic.Bool
	stopped chan struct{}
}

func (c *secureCaptureTest) SourceCapture() wininput.SourceActivation { return c }
func (c *secureCaptureTest) Active() bool                             { return c.active.Load() }
func (c *secureCaptureTest) Activate() error                          { c.active.Store(true); return nil }
func (c *secureCaptureTest) Deactivate()                              { c.active.Store(false) }
func (c *secureCaptureTest) AdmitBoundary(context.Context, control.BoundaryIntent) (peer.BoundaryAdmissionDecision, error) {
	return peer.BoundaryAdmissionAllowed, nil
}
func (c *secureCaptureTest) RunWithBoundary(ctx context.Context, _ *wininput.SourceSession, _ wininput.BoundaryObserver, ready func()) error {
	ready()
	<-ctx.Done()
	close(c.stopped)
	return ctx.Err()
}

func TestLockedSourceUsesProtectedCaptureInSameSessionAndFencesActiveRefresh(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	secure := &secureCaptureTest{stopped: make(chan struct{})}
	h := &protectedSource{blocked: func() bool { return true }, native: protectedTestHook(func(context.Context, *wininput.SourceSession, wininput.BoundaryObserver, func()) error {
		t.Error("medium capture attempted on secure desktop")
		return errors.New("wrong owner")
	})}
	h.SetProtectedSource(secure)
	ready, done := make(chan struct{}), make(chan error, 1)
	session := &wininput.SourceSession{}
	go func() { done <- h.RunWithBoundary(ctx, session, nil, func() { close(ready) }) }()
	<-ready
	decision, err := h.AdmitBoundary(ctx, control.BoundaryIntent{})
	if err != nil || decision != peer.BoundaryAdmissionAllowed {
		t.Fatal("secure native admission unavailable", err)
	}
	if err := h.SourceCapture().Activate(); err != nil || !secure.Active() {
		t.Fatal("protected activation failed", err)
	}
	if h.refreshDesktop("desktop_switch") {
		t.Fatal("active protected owner replaced")
	}
	h.SourceCapture().Deactivate()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-secure.stopped:
	default:
		t.Fatal("owner was not joined")
	}
}
