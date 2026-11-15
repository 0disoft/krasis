package wininput

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/control"
	"testing"
)

type heightCursorPort struct {
	x, y              int32
	reads, writes     int
	readErr, writeErr error
}

func (p *heightCursorPort) CursorPosition(context.Context) (int32, int32, error) {
	p.reads++
	return p.x, p.y, p.readErr
}
func (p *heightCursorPort) SetCursorPosition(_ context.Context, x, y int32) error {
	p.writes++
	p.x, p.y = x, y
	return p.writeErr
}

func TestCursorHeightPlacementAndReturn(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeLeft, control.ScreenEdgeRight} {
		for _, y := range []int32{0, 1, 539, 540, 541, 1078, 1079} {
			h := control.NormalizeEdgeHeight(y, 0, 1080)
			x, targetY, err := cursorEntryPoint(edge, -1920, -100, 1920, 1440, h)
			if err != nil {
				t.Fatal(err)
			}
			wantY, _ := h.Coordinate(-100, 1440)
			wantX := int32(-1919)
			if edge == control.ScreenEdgeLeft {
				wantX = -2
			}
			if x != wantX || targetY != wantY {
				t.Fatal("entry lost height", x, targetY, wantX, wantY)
			}
			p := &heightCursorPort{x: 1919, y: 550}
			if err := restoreCursorHeight(context.Background(), p, h, 0, 1080); err != nil || p.x != 1919 || p.y != y || p.reads != 1 || p.writes != 1 {
				t.Fatal("return reused old coordinate", p, err)
			}
		}
	}
}

func TestCursorHeightReturnFailures(t *testing.T) {
	for _, h := range []control.EdgeHeight{0, 65537} {
		p := &heightCursorPort{}
		err := restoreCursorHeight(context.Background(), p, h, 0, 1080)
		if (err != nil) != (h != 0) || p.reads != 0 || p.writes != 0 {
			t.Fatal("invalid or legacy height performed I/O", p, err)
		}
	}
	cause := errors.New("cursor unavailable")
	p := &heightCursorPort{readErr: cause}
	if err := restoreCursorHeight(context.Background(), p, 1, 0, 1080); !errors.Is(err, cause) || p.writes != 0 {
		t.Fatal("read error swallowed or wrote cursor", err)
	}
	p = &heightCursorPort{writeErr: cause}
	if err := restoreCursorHeight(context.Background(), p, 1, 0, 1080); !errors.Is(err, cause) || p.writes != 1 {
		t.Fatal("write error lost or replayed", err)
	}
}

func TestReturnIntentHeightDoesNotChangeLeaseIdentity(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeLeft, control.ScreenEdgeRight} {
		for _, y := range []int32{-100, 500, 1339} {
			a, err := newInputAdapterWithReturnRearmDistance(10)
			if err != nil {
				t.Fatal(err)
			}
			a.setReturnGeometry(-100, 1440)
			base := control.ReturnIntent{SessionID: "s", Epoch: "e"}
			if err := a.ArmReturn(base, edge); err != nil {
				t.Fatal(err)
			}
			if err := a.observeReturnPosition(500, 0, 1000, y); err != nil {
				t.Fatal(err)
			}
			x := int32(0)
			if edge == control.ScreenEdgeLeft {
				x = 999
			}
			if err := a.observeReturnPosition(x, 0, 1000, y); err != nil {
				t.Fatal(err)
			}
			got := <-a.ReturnIntents()
			if got.Height != control.NormalizeEdgeHeight(y, -100, 1440) {
				t.Fatal("return lost current crossing height", got)
			}
			a.RearmReturn(got)
			if !a.returns.armed {
				t.Fatal("height broke rearm matching")
			}
			a.DisarmReturn(got)
			if a.returns.armed || a.returns.intent.SessionID != "" {
				t.Fatal("height broke disarm matching")
			}
		}
	}
}
