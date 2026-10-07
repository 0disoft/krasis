//go:build windows && desktop

package workspacehost

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"testing"
)

func TestWorkspaceWindowFitsScaledWorkAreasAndTaskbarOffsets(t *testing.T) {
	for _, work := range []application.Rect{
		{Width: 1920, Height: 1032},
		{Width: 1536, Height: 816},                 // 1920x1080 at 125%, bottom taskbar excluded.
		{X: 0, Y: 40, Width: 1280, Height: 680},    // 150%, top taskbar.
		{X: -960, Y: -30, Width: 960, Height: 510}, // 200%, secondary monitor.
		{X: 48, Y: 0, Width: 863, Height: 464},     // 150% on a smaller laptop, side taskbar.
		{Width: 320, Height: 200},
	} {
		for _, initial := range []bool{true, false} {
			bounds, mw, mh := workspaceWindowBounds(work, application.Rect{X: -5000, Y: 5000, Width: 1180, Height: 850}, initial)
			if bounds.X < work.X || bounds.Y < work.Y || bounds.X+bounds.Width > work.X+work.Width || bounds.Y+bounds.Height > work.Y+work.Height {
				t.Fatalf("window outside work area: work=%+v bounds=%+v", work, bounds)
			}
			if mw > bounds.Width || mh > bounds.Height {
				t.Fatalf("minimum size forces overflow: %d %d %+v", mw, mh, bounds)
			}
		}
	}
}

func TestWorkspaceReopenKeepsUserPlacementWhenItFits(t *testing.T) {
	work := application.Rect{Width: 1920, Height: 1032}
	current := application.Rect{X: 90, Y: 80, Width: 860, Height: 550}
	got, _, _ := workspaceWindowBounds(work, current, false)
	if got != current {
		t.Fatal("reopening resized or centered an already fitting window", got)
	}
}
