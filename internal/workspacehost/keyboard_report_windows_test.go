//go:build windows && desktop

package workspacehost

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/keyboardstate"
)

func TestKeyboardReportsWithoutRendererAndStopsOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reports := make(chan keyboardstate.State, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runKeyboardReports(ctx, func(ctx context.Context) (keyboardstate.State, bool) {
			if _, ok := ctx.Deadline(); !ok {
				panic("unbounded native observation")
			}
			return keyboardstate.Unknown(), true
		}, func(_ context.Context, state keyboardstate.State) { reports <- state })
	}()
	select {
	case state := <-reports:
		if state != keyboardstate.Unknown() {
			t.Fatal("background state was invented")
		}
	case <-time.After(time.Second):
		t.Fatal("native reporter depended on renderer polling")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reporter outlived shutdown")
	}
}

func TestFailedKeyboardSampleIsNotPublishedAsFreshState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := false
	runKeyboardReports(ctx, func(context.Context) (keyboardstate.State, bool) {
		cancel()
		return keyboardstate.State{Caps: "ON", Num: "ON", Hangul: "ON"}, false
	}, func(context.Context, keyboardstate.State) { called = true })
	if called {
		t.Fatal("failed sample was published")
	}
}
