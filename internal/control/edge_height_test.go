package control

import (
	"math"
	"testing"
)

func TestEdgeHeightAcrossDifferentScreens(t *testing.T) {
	for _, geometry := range [][2]int32{{0, 1080}, {-1200, 1200}, {200, 2160}, {0, 1}} {
		top, height := geometry[0], geometry[1]
		for _, offset := range []int32{0, 1, height/2 - 1, height / 2, height/2 + 1, height - 2, height - 1} {
			y := max(top, min(top+offset, top+height-1))
			h := NormalizeEdgeHeight(y, top, height)
			got, err := h.Coordinate(top, height)
			if err != nil || got != y {
				t.Fatalf("geometry %v y=%d anchor=%d got=%d err=%v", geometry, y, h, got, err)
			}
		}
	}
	for _, test := range []struct {
		y    int32
		want EdgeHeight
	}{{-1, 1}, {0, 1}, {1079, 65536}, {1080, 65536}} {
		if got := NormalizeEdgeHeight(test.y, 0, 1080); got != test.want {
			t.Fatalf("y=%d got=%d want=%d", test.y, got, test.want)
		}
	}
	for _, h := range []EdgeHeight{1, 32769, 65536} {
		y, err := h.Coordinate(-100, 1441)
		want := map[EdgeHeight]int32{1: -100, 32769: 620, 65536: 1340}[h]
		if err != nil || y != want {
			t.Fatalf("anchor=%d y=%d want=%d err=%v", h, y, want, err)
		}
	}
}

func TestEdgeHeightInvalidAndLegacy(t *testing.T) {
	if y, err := EdgeHeight(0).Coordinate(0, 1080); err != nil || y != 540 {
		t.Fatal(y, err)
	}
	for _, test := range []struct {
		h           EdgeHeight
		top, height int32
	}{{65537, 0, 1080}, {1, 0, 0}, {1, 0, -1}, {1, math.MaxInt32, 2}} {
		if _, err := test.h.Coordinate(test.top, test.height); err == nil {
			t.Fatalf("accepted %+v", test)
		}
	}
	if NormalizeEdgeHeight(0, math.MaxInt32, 2) != 0 || NormalizeEdgeHeight(0, 0, 0) != 0 {
		t.Fatal("invalid geometry accepted")
	}
}
