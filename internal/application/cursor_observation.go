package application

const (
	CursorTargetAdjusted uint8 = 1 << iota
	CursorReadbackMatches
	CursorClipChanged
	CursorGeometryChanged
	CursorPhysicalMatchesTarget
	CursorPhysicalMatchesReadback
)

// CursorPlacementObservation contains only comparisons. Unknown queries are
// absent from Known, not false. It contains no positions, rectangles or IDs.
type CursorPlacementObservation struct {
	Known uint8 `json:"known"`
	Flags uint8 `json:"flags"`
}

func (p CursorPlacementObservation) Valid() bool {
	const core = CursorTargetAdjusted | CursorReadbackMatches
	const physical = CursorPhysicalMatchesTarget | CursorPhysicalMatchesReadback
	if p.Known&^uint8(63) != 0 || p.Flags&^p.Known != 0 || p.Known&core != core {
		return false
	}
	if p.Known&physical != 0 && p.Known&physical != physical {
		return false
	}
	if p.Flags&CursorReadbackMatches != 0 {
		return p.Known == core
	}
	// A single physical read cannot equal two distinct target/readback points.
	return p.Flags&physical != physical
}

func (p *CursorPlacementObservation) Record(bit uint8, value bool) {
	if p == nil || bit == 0 || bit&^uint8(63) != 0 || bit&(bit-1) != 0 || p.Known&bit != 0 {
		return
	}
	p.Known |= bit
	if value {
		p.Flags |= bit
	}
}
