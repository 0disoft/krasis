package application

import "context"

// CursorPositionPort is local cursor I/O on the selected input backend. It
// grants no lease authority and never selects a process, session or desktop.
type CursorPositionPort interface {
	CursorPosition(context.Context) (int32, int32, error)
	SetCursorPosition(context.Context, int32, int32) error
}
