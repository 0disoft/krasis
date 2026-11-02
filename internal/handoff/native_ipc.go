package handoff

// NativeIPCProfile splits one matching local child exchange. The spans are:
// executor admission, request-to-child-read, child admission, child handler,
// reply-ready-to-first-read-return, and first-read-return-to-matched-response.
// Reply-ready does not prove kernel arrival or Write completion. Only durations
// survive normalization; no raw counter, input, identity or extra probe does.
type NativeIPCProfile struct {
	Micros [6]uint32 `json:"micros"`
}

func (p NativeIPCProfile) Valid() bool {
	return p.total() <= uint64(Window.Microseconds())
}

func (p NativeIPCProfile) total() uint64 {
	var total uint64
	for _, n := range p.Micros {
		total += uint64(n)
	}
	return total
}
