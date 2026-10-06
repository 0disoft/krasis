package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

func TestSourceFocusReturnReusesAuthenticatedConnection(t *testing.T) {
	left, right := openConnectionPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 2)
	received := make(chan struct{})
	returned := make(chan struct{})
	hook := &focusReturnHook{received: received, returned: returned}
	rightInput := newTestInput(false)
	config := DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10})
	go func() {
		_, err := RunSession(ctx, config, Dependencies{Connection: left, Hook: hook, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}})
		done <- err
	}()
	go func() {
		_, err := RunSession(ctx, config, Dependencies{Connection: right, Hook: &idleHook{}, Input: rightInput, Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}})
		done <- err
	}()
	for range 2 {
		select {
		case <-rightInput.moves:
			received <- struct{}{}
		case err := <-done:
			t.Fatal("connection ended before repeat crossing", err)
		case <-ctx.Done():
			t.Fatal("repeat crossing timed out")
		}
	}
	select {
	case <-returned:
	case <-ctx.Done():
		t.Fatal("local return did not complete")
	}
	cancel()
	for range 2 {
		if err := <-done; !errors.Is(err, context.Canceled) && !errors.Is(err, peer.ErrPeerRemoteEOF) {
			t.Fatal(err)
		}
	}
}

type focusReturnHook struct{ received, returned chan struct{} }

func (hook *focusReturnHook) RunWithBoundary(ctx context.Context, session *wininput.SourceSession, boundary wininput.BoundaryObserver, ready func()) error {
	ready()
	wait := func(condition func() bool) error {
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for !condition() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
		return nil
	}
	for range 2 {
		for _, x := range []int32{50, 99} {
			if err := boundary.Observe(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, x); err != nil {
				return err
			}
		}
		if err := wait(session.Active); err != nil {
			return err
		}
		if !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
			return errors.New("remote input not accepted")
		}
		select {
		case <-hook.received:
		case <-ctx.Done():
			return ctx.Err()
		}
		if !session.RequestLocalReturn() {
			return errors.New("local return not accepted")
		}
		if err := wait(func() bool { return !session.LocalReturnPending() }); err != nil {
			return err
		}
	}
	close(hook.returned)
	<-ctx.Done()
	return ctx.Err()
}
