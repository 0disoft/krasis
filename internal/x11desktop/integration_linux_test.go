package x11desktop

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestX11NativeRoundTrip(t *testing.T) {
	if os.Getenv("KRASIS_X11_TEST") != "1" {
		t.Skip("requires isolated Xvfb DISPLAY; injects input")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, err := Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.SetPosition(ctx, 100, 100); err != nil {
		t.Fatal(err)
	}
	x, y, err := d.Move(ctx, 17, -9)
	if err != nil || x != 117 || y != 91 {
		t.Fatalf("relative move %d,%d: %v", x, y, err)
	}
	if err = d.Key(ctx, 0x1e, true); err != nil {
		t.Fatal(err)
	}
	held, err := d.AnyPressed(ctx)
	if err != nil || !held {
		t.Fatalf("key not held: %v", err)
	}
	if err = d.Key(ctx, 0x1e, false); err != nil {
		t.Fatal(err)
	}
	held, err = d.AnyPressed(ctx)
	if err != nil || held {
		t.Fatalf("key not released: %v", err)
	}
	for _, code := range []uint32{1, 2, 3, 4, 5} {
		if err = d.Button(ctx, code, true); err != nil {
			t.Fatal(err)
		}
		held, err = d.AnyPressed(ctx)
		if err != nil || !held {
			t.Fatalf("button %d admission missed held state: %v", code, err)
		}
		if err = d.Button(ctx, code, false); err != nil {
			t.Fatal(err)
		}
	}
	if err = d.Wheel(ctx, 60, 60); err != nil {
		t.Fatal(err)
	}
	if err = d.Wheel(ctx, 60, 60); err != nil {
		t.Fatal(err)
	}
	if err = d.Grab(ctx); err != nil {
		t.Fatal(err)
	}
	if err = d.Ungrab(ctx); err != nil {
		t.Fatal(err)
	}
	probe, err := Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if err = probe.Grab(ctx); err != nil {
		t.Fatalf("capture leaked: %v", err)
	}
	if err = probe.Ungrab(ctx); err != nil {
		t.Fatal(err)
	}
}
