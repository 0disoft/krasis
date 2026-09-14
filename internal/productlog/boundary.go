package productlog

// BoundarySummary contains geometry and aggregate detector decisions, never an
// input stream. A candidate is not proof of a completed control handoff.
type BoundarySummary struct {
	Left           int32  `json:"left"`
	Top            int32  `json:"top"`
	Width          int32  `json:"width"`
	Height         int32  `json:"height"`
	PhysicalWidth  uint32 `json:"physical_width"`
	PhysicalHeight uint32 `json:"physical_height"`
	DPIAwareness   int32  `json:"dpi_awareness"`
	Edge           string `json:"edge"`
	Gap            int32  `json:"gap"`
	Rearm          int32  `json:"rearm"`
	Candidates     uint64 `json:"candidates"`
	LastOffset     int64  `json:"last_offset"`
	Complete       bool   `json:"complete"`
}

func (s BoundarySummary) valid() bool {
	return s.Width > 1 && s.Width <= 131072 && s.Height > 0 && s.Height <= 131072 &&
		s.Left >= -131072 && s.Left <= 131072 && s.Top >= -131072 && s.Top <= 131072 &&
		s.PhysicalWidth <= 131072 && s.PhysicalHeight <= 131072 &&
		s.DPIAwareness >= -1 && s.DPIAwareness <= 2 && (s.Edge == "LEFT" || s.Edge == "RIGHT") &&
		s.Gap >= 0 && s.Gap < s.Width && s.Rearm > s.Gap && s.Rearm < s.Width &&
		s.LastOffset >= -4294967295 && s.LastOffset <= 4294967295 && (s.Candidates != 0 || s.LastOffset == 0)
}

func (l *Logger) Boundary(s BoundarySummary) {
	if !s.valid() {
		return
	}
	l.enqueue("boundary_summary", &s)
}
