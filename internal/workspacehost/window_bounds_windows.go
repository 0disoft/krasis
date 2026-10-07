//go:build windows && desktop

package workspacehost

import "github.com/wailsapp/wails/v3/pkg/application"

// WorkArea and window bounds use Wails DIP coordinates, not physical pixels.
// Keep a margin around a normal window and let small displays override the
// usual minimum size so neither the titlebar nor the bottom is unreachable.
func workspaceWindowBounds(work, current application.Rect, initial bool) (application.Rect, int, int) {
	margin := min(24, min(work.Width, work.Height)/20)
	width, height := max(1, work.Width-2*margin), max(1, work.Height-2*margin)
	minWidth, minHeight := min(720, width), min(480, height)
	if initial || current.Width <= 0 || current.Height <= 0 {
		current.Width, current.Height = min(1000, width), min(640, height)
		current.X = work.X + (work.Width-current.Width)/2
		current.Y = work.Y + (work.Height-current.Height)/2
	} else {
		current.Width = min(max(minWidth, current.Width), width)
		current.Height = min(max(minHeight, current.Height), height)
		current.X = max(work.X+margin, min(current.X, work.X+work.Width-margin-current.Width))
		current.Y = max(work.Y+margin, min(current.Y, work.Y+work.Height-margin-current.Height))
	}
	return current, minWidth, minHeight
}

func fitWorkspaceWindow(window *application.WebviewWindow, initial bool) {
	if window == nil || window.IsMaximised() || window.IsFullscreen() {
		return
	}
	screen, err := window.GetScreen()
	if err != nil || screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
		return
	}
	bounds, minWidth, minHeight := workspaceWindowBounds(screen.WorkArea, window.Bounds(), initial)
	window.SetMinSize(minWidth, minHeight)
	window.SetBounds(bounds)
}
