package x11desktop

import (
	"context"
	"errors"
	"testing"
)

func TestPhysicalNamesPreserveScanCodes(t *testing.T) {
	for name, want := range map[string]uint32{"AD01": 0x10, "AC01": 0x1e, "AB01": 0x2c, "AE12": 0x0d, "RCTL": 0x1001d, "RTRN": 0x1c, "KPEN": 0x1001c, "FK10": 68, "FK11": 87, "FK12": 88, "UP\x00\x00": 0x10048, "ESC\x00": 1} {
		got, ok := scanForName(name)
		if !ok || got != want {
			t.Errorf("%q=%x,%v want %x", name, got, ok, want)
		}
	}
	for _, name := range []string{"AD00", "AE13bad", "AD13", "FK13", "XXXX"} {
		if _, ok := scanForName(name); ok {
			t.Errorf("accepted %s", name)
		}
	}
}
func TestKeyMapBoundsAndDuplicates(t *testing.T) {
	f, r, err := keyMaps(8, []byte("ESC\x00RCTLAD01"))
	if err != nil {
		t.Fatal(err)
	}
	if f[9] != 0x1001d || r[0x1001d] != 9 {
		t.Fatal("extended identity lost")
	}
	for _, tc := range []struct {
		first int
		names string
	}{{7, "ESC\x00"}, {255, "ESC\x00AD01"}, {8, "ESC"}, {8, "ESC\x00ESC\x00"}, {8, "XXXX"}} {
		if _, _, err := keyMaps(tc.first, []byte(tc.names)); err == nil {
			t.Fatalf("accepted invalid names %+v", tc)
		}
	}
}
func TestLocalDisplayBounds(t *testing.T) {
	for _, s := range []string{":0", ":91.0", ":65535.255"} {
		if _, _, err := localDisplay(s); err != nil {
			t.Error(s, err)
		}
	}
	for _, s := range []string{"", ":-1", ":65536", ":1.256", ":1.-1", ":1.0.0", "localhost:1", "tcp/host:1", ":../1"} {
		if _, _, err := localDisplay(s); err == nil {
			t.Error("accepted", s)
		}
	}
}
func TestOpenCancellationAndWaylandRefusal(t *testing.T) {
	t.Setenv("DISPLAY", ":9999")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("XDG_SESSION_TYPE", "x11")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	if _, err := Open(context.Background()); !errors.Is(err, ErrWayland) {
		t.Fatalf("XWayland accepted: %v", err)
	}
}
