package wininput

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/x11desktop"
)

func TestLinuxReleaseRecoveryRejectsCanceledClosedAndUnrelatedFailures(t *testing.T) {
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 0x1e}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	other := errors.New("native server rejected input")
	for _, tc := range []struct {
		ctx    context.Context
		closed bool
		cause  error
	}{{nil, false, net.ErrClosed}, {canceled, false, io.EOF}, {context.Background(), true, net.ErrClosed}, {context.Background(), false, other}, {context.Background(), false, nil}} {
		p := &linuxInjection{closed: tc.closed}
		err := p.recoverRelease(tc.ctx, token, tc.cause)
		if err != tc.cause || p.releaseDesktop != nil {
			t.Fatalf("unexpected recovery: %v, owner=%v", err, p.releaseDesktop)
		}
	}
}

func TestLinuxNativeReleaseRecoversClosedConnectionWithoutRearmingInput(t *testing.T) {
	if os.Getenv("KRASIS_X11_TEST") != "1" {
		t.Skip("requires isolated Xvfb DISPLAY; injects input")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probe, err := x11desktop.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	tokens := []control.PressedToken{{Kind: control.PressedKeyboardKey, Code: 0x1e}, {Kind: control.PressedMouseButton, Code: MouseButtonLeft}}
	p := &linuxInjection{}
	defer p.Close()
	// A separate healthy owner also releases tokens if this assertion fails.
	defer func() {
		_ = probe.Key(context.Background(), 0x1e, false)
		_ = probe.Button(context.Background(), MouseButtonLeft, false)
	}()
	for _, token := range tokens {
		if err := p.Press(ctx, token); err != nil {
			t.Fatal(err)
		}
	}
	if held, err := probe.AnyPressed(ctx); err != nil || !held {
		t.Fatal("tokens not held", held, err)
	}
	_ = p.desktop.Close() // deterministic counterpart of cancellation closing X11
	for _, token := range tokens {
		if err := p.Release(ctx, token); err != nil {
			t.Fatal("release did not recover", err)
		}
	}
	if held, err := probe.AnyPressed(ctx); err != nil || held {
		t.Fatal("native tokens retained", held, err)
	}
	if p.releaseDesktop == nil || p.releaseDesktop == p.desktop {
		t.Fatal("recovery reused poisoned owner")
	}
	if err := p.Press(ctx, tokens[0]); err == nil {
		t.Fatal("recovery rearmed key presses")
	}
	if err := p.Move(ctx, 1, 0); err == nil {
		t.Fatal("recovery rearmed motion")
	}
	_ = p.releaseDesktop.Close()
	if err := p.Release(ctx, tokens[0]); err == nil {
		t.Fatal("failed recovery owner reported successful release")
	}
	_ = p.Close()
	if err := p.Release(ctx, tokens[0]); err == nil {
		t.Fatal("closed input owner reopened recovery")
	}
}
