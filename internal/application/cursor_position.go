package application

import "context"

// CursorPositionPort is local cursor I/O on the selected input backend. It
// grants no lease authority and never selects a process, session or desktop.
type CursorPositionPort interface {
	CursorPosition(context.Context) (int32, int32, error)
	SetCursorPosition(context.Context, int32, int32) error
}

// PositionedMovePort keeps motion and its return-boundary snapshot in one
// backend operation, avoiding an extra worker roundtrip per pointer event.
type PositionedMovePort interface {
	MoveWithPosition(context.Context, int32, int32) (int32, int32, error)
}

// CursorPlacement carries the backend's independently computed allowed target
// and its readback. They are private cursor state, never diagnostic fields.
// The caller must compare the two before committing an entry.
type CursorPlacement struct {
	TargetX, TargetY int32
	X, Y             int32
}

// CursorPlacementPort places and inspects the cursor in one authorized backend
// operation. It respects existing confinement without changing ClipCursor.
type CursorPlacementPort interface {
	PlaceCursor(context.Context, int32, int32) (CursorPlacement, error)
}
