package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

type linuxTestPort struct {
	x, y   int32
	writes int
	fail   error
}

func (p *linuxTestPort) Press(context.Context, control.PressedToken) error   { return nil }
func (p *linuxTestPort) Release(context.Context, control.PressedToken) error { return nil }
func (p *linuxTestPort) Move(context.Context, int32, int32) error            { return nil }
func (p *linuxTestPort) Wheel(context.Context, int32, int32) error           { return nil }
func (p *linuxTestPort) MoveWithPosition(_ context.Context, x, y int32) (int32, int32, error) {
	p.x += x
	p.y += y
	return p.x, p.y, nil
}
func (p *linuxTestPort) CursorPosition(context.Context) (int32, int32, error) { return p.x, p.y, nil }
func (p *linuxTestPort) SetCursorPosition(_ context.Context, x, y int32) error {
	p.writes++
	if p.fail != nil {
		return p.fail
	}
	p.x, p.y = x, y
	return nil
}
func (p *linuxTestPort) displayGeometry(context.Context) (DisplayGeometry, error) {
	return DisplayGeometry{Count: 1, Width: 100, Height: 80}, nil
}

func TestLinuxCursorEntryRollbackAndLocalMovement(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeLeft, control.ScreenEdgeRight} {
		t.Run(string(edge), func(t *testing.T) {
			p := &linuxTestPort{x: 25, y: 30}
			a, err := NewInputAdapterWithInjection(10, 100, p)
			if err != nil {
				t.Fatal(err)
			}
			if err = a.PrepareEntry(context.Background(), edge); err != nil {
				t.Fatal(err)
			}
			wantX := int32(1)
			if edge == control.ScreenEdgeLeft {
				wantX = 98
			}
			if p.x != wantX || p.y != 40 {
				t.Fatal(p.x, p.y)
			}
			if err = a.PrepareEntry(context.Background(), edge); !errors.Is(err, ErrCursorEntryState) {
				t.Fatal("pending entry replaced", err)
			}
			outcome, err := a.RollbackEntry(context.Background())
			if err != nil || outcome != application.CursorRollbackRestored || p.x != 25 || p.y != 30 {
				t.Fatal(outcome, err, p.x, p.y)
			}
			if err = a.PrepareEntry(context.Background(), edge); err != nil {
				t.Fatal(err)
			}
			p.x = 50
			outcome, err = a.RollbackEntry(context.Background())
			if err != nil || outcome != application.CursorRollbackNotRestored || p.x != 50 {
				t.Fatal("overwrote local movement", outcome, err)
			}
		})
	}
}
func TestLinuxCursorCancellationAndUncertainWrite(t *testing.T) {
	p := &linuxTestPort{x: 25, y: 30}
	a, _ := NewInputAdapterWithInjection(10, 100, p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.PrepareEntry(ctx, control.ScreenEdgeRight); !errors.Is(err, context.Canceled) || p.writes != 0 {
		t.Fatal("canceled entry performed IO", err)
	}
	p.fail = errors.New("lost write reply")
	if err := a.PrepareEntry(context.Background(), control.ScreenEdgeRight); !errors.Is(err, p.fail) {
		t.Fatal(err)
	}
	if _, err := a.RollbackEntry(context.Background()); !errors.Is(err, ErrCursorEntryState) {
		t.Fatal("unknown write reported safe", err)
	}
}
func TestLinuxMouseWireMapping(t *testing.T) {
	for b, want := range map[byte]uint32{1: MouseButtonLeft, 2: MouseButtonMiddle, 3: MouseButtonRight, 8: MouseButtonX1, 9: MouseButtonX2} {
		a, emit, _, err := linuxButtonAction(b, true)
		if err != nil || !emit || a.Kind != control.InputButtonDown || a.Code != want {
			t.Fatal(b, a, emit, err)
		}
	}
	for _, b := range []byte{4, 5, 6, 7} {
		a, emit, _, err := linuxButtonAction(b, true)
		if err != nil || !emit || a.Kind != control.InputWheel || a.Validate() != nil {
			t.Fatal(b, a, err)
		}
		if _, emit, _, _ := linuxButtonAction(b, false); emit {
			t.Fatal("wheel release duplicated", b)
		}
	}
}
