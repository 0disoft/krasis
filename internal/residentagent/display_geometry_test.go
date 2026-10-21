package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/wininput"
	"sync/atomic"
	"testing"
)

func TestGeometryChangePermanentlyRetiresPreparedConfiguration(t *testing.T) {
	initial := wininput.DisplayGeometry{Count: 1, Width: 1920, Height: 1080}
	for _, changed := range []wininput.DisplayGeometry{{Count: 1, Width: 1080, Height: 1920}, {Count: 2, Width: 3840, Height: 1080}, {Count: 1, Left: -1920, Width: 1920, Height: 1080}, {}} {
		current := initial
		stopped := 0
		check := displayGeometryGuard(context.Background(), initial, func() wininput.DisplayGeometry { return current }, func() { stopped++ })
		if err := check(); err != nil {
			t.Fatal(err)
		}
		current = changed
		if err := check(); !errors.Is(err, wininput.ErrDisplayGeometryChanged) {
			t.Fatalf("changed geometry accepted: %v", err)
		}
		current = initial
		if err := check(); !errors.Is(err, wininput.ErrDisplayGeometryChanged) || stopped != 1 {
			t.Fatal("old geometry owner revived")
		}
		fresh := displayGeometryGuard(context.Background(), initial, func() wininput.DisplayGeometry { return current }, func() { t.Fatal("fresh geometry rejected") })
		if err := fresh(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGeometryGuardRejectsUnsupportedInitialConfiguration(t *testing.T) {
	g := wininput.DisplayGeometry{Count: 2, Width: 1920, Height: 1080}
	stopped := false
	check := displayGeometryGuard(context.Background(), g, func() wininput.DisplayGeometry { return g }, func() { stopped = true })
	if !errors.Is(check(), wininput.ErrDisplayGeometryChanged) || !stopped {
		t.Fatal("unsupported display admitted")
	}
}

func TestGeometryGuardRejectsCancellationDuringNativeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g := wininput.DisplayGeometry{Count: 1, Width: 1920, Height: 1080}
	check := displayGeometryGuard(ctx, g, func() wininput.DisplayGeometry { cancel(); return g }, func() { t.Fatal("unchanged geometry was invalidated") })
	if err := check(); !errors.Is(err, context.Canceled) {
		t.Fatal("late geometry result admitted canceled session", err)
	}
}

func TestGeometryGuardCannotReviveAfterConcurrentInvalidation(t *testing.T) {
	g := wininput.DisplayGeometry{Count: 1, Width: 1920, Height: 1080}
	entered, release := make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	check := displayGeometryGuard(context.Background(), g, func() wininput.DisplayGeometry {
		if reads.Add(1) == 1 {
			close(entered)
			<-release
			return g
		}
		return wininput.DisplayGeometry{}
	}, func() {})
	first := make(chan error, 1)
	go func() { first <- check() }()
	<-entered
	second := check()
	close(release)
	if !errors.Is(second, wininput.ErrDisplayGeometryChanged) {
		t.Fatal("display change was accepted", second)
	}
	if err := <-first; !errors.Is(err, wininput.ErrDisplayGeometryChanged) {
		t.Fatal("late native result revived invalidated geometry", err)
	}
}
