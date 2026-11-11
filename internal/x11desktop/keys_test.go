package x11desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func legacyScanForName(name string) (uint32, bool) {
	name = strings.TrimRight(name, "\x00 ")
	if code, ok := physicalKeys[name]; ok {
		return code, true
	}
	for _, row := range []struct {
		prefix            string
		first, last, scan int
	}{
		{"AE", 1, 12, 2}, {"AD", 1, 12, 16}, {"AC", 1, 11, 30},
		{"AB", 1, 10, 44}, {"FK", 1, 10, 59}, {"FK", 11, 12, 87},
	} {
		for n := row.first; n <= row.last; n++ {
			if name == fmt.Sprintf("%s%02d", row.prefix, n) {
				return uint32(row.scan + n - row.first), true
			}
		}
	}
	return 0, false
}

func TestPhysicalNameRowsMatchPreviousMapping(t *testing.T) {
	for _, prefix := range []string{"AE", "AD", "AC", "AB", "FK", "ZZ"} {
		for n := 0; n < 100; n++ {
			name := fmt.Sprintf("%s%02d", prefix, n)
			for _, suffix := range []string{"", "\x00", " "} {
				got, ok := scanForName(name + suffix)
				want, accepted := legacyScanForName(name + suffix)
				if ok != accepted || got != want {
					t.Fatalf("%q: got %x,%v want %x,%v", name+suffix, got, ok, want, accepted)
				}
			}
		}
	}
	for name, want := range map[string]uint32{"KP0": 82, "KP1": 79, "KP2": 80, "KP3": 81, "KP4": 75, "KP5": 76, "KP6": 77, "KP7": 71, "KP8": 72, "KP9": 73, "KPDL": 83} {
		if got, ok := scanForName(name); !ok || got != want {
			t.Fatalf("keypad %s: %x,%v", name, got, ok)
		}
	}
	for _, name := range []string{"AD-1", "AD+1", "AD1", "AD001", "AD\xff1", "FK1x"} {
		if _, ok := scanForName(name); ok {
			t.Fatalf("accepted malformed name %q", name)
		}
	}
}

func BenchmarkPhysicalNameDecode(b *testing.B) {
	for _, tc := range []struct {
		name   string
		decode func(string) (uint32, bool)
	}{{"previous", legacyScanForName}, {"current", scanForName}} {
		b.Run(tc.name, func(b *testing.B) {
			names := []string{"AD12", "AC11", "AB10", "FK12", "ZZZZ"}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				tc.decode(names[i%len(names)])
			}
		})
	}
}

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
