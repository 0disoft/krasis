package inputworker

import (
	"io"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

// Delegation preserves Read sizes, counts, bytes and errors. A positive return
// is client-observed progress even when it comes with EOF. No probe read,
// goroutine, input retry, deadline update or worker protocol is introduced.
type ackReadObserver struct {
	reader                 io.Reader
	profile                *handoff.ACKReadProfile
	started, first, header time.Time
}

func (r *ackReadObserver) Read(data []byte) (int, error) {
	n, err := r.reader.Read(data)
	if n > 0 && r.first.IsZero() {
		r.first = time.Now()
		r.profile.Record(0, r.first.Sub(r.started))
		r.profile.Progress = handoff.ACKReadData
	}
	return n, err
}

func (r *ackReadObserver) headerResult(decoded bool) {
	if r == nil {
		return
	}
	at := time.Now()
	if r.first.IsZero() {
		r.profile.Record(0, at.Sub(r.started))
	} else {
		r.profile.Record(1, at.Sub(r.first))
	}
	if decoded {
		r.header = at
		r.profile.Progress = handoff.ACKReadHeader
	}
}

func (r *ackReadObserver) suffixResult(decoded bool) {
	if r == nil {
		return
	}
	r.profile.Record(2, time.Since(r.header))
	if decoded {
		r.profile.Progress = handoff.ACKReadComplete
	}
}
