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
	Observation      *CursorPlacementObservation
}

// CursorPlacementPort places and inspects the cursor in one authorized backend
// operation. It respects existing confinement without changing ClipCursor.
type CursorPlacementPort interface {
	PlaceCursor(context.Context, int32, int32) (CursorPlacement, error)
}

// CursorEntry includes the pre-write snapshot for conditional rollback. The
// installed backend returns it with placement in one authenticated transaction.
// These coordinates stay private to the input channel, never diagnostics.
type CursorEntry struct {
	CursorPlacement
	BeforeX, BeforeY int32
}

type CursorEntryPort interface {
	EnterCursor(context.Context, int32, int32) (CursorEntry, error)
}

// CursorPlacementProbePort adds bounded, read-only mismatch classification to
// the same placement operation. It never retries or changes placement policy.
type CursorPlacementProbePort interface {
	ProbeCursorPlacement(context.Context, int32, int32) (CursorPlacement, error)
}
