package inputworker

import (
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// The query owner must retain its exact native pipe until actual return, but
// retiring that proof must not close the independent final-report transport.
// There are exactly two references: authority queries and bootstrap reporting.
// Neither reference grants input authority or changes the channel's fence.
func sourceReportChannel(control net.Conn, proof *Authority) (*Channel, io.Closer) {
	owner := &sourceControlOwner{conn: control}
	owner.references.Store(2)
	query := &sourceControlReference{Conn: control, owner: owner}
	report := &sourceControlReference{Conn: control, owner: owner}
	return &Channel{stream: query, authority: proof}, report
}

type sourceControlOwner struct {
	conn       net.Conn
	references atomic.Int32
}

type sourceControlReference struct {
	net.Conn
	owner *sourceControlOwner
	once  sync.Once
	err   error
}

func (ref *sourceControlReference) Close() error {
	ref.once.Do(func() {
		if ref.owner.references.Add(-1) == 0 {
			ref.err = ref.owner.conn.Close()
		}
	})
	return ref.err
}

// workerControlLifetime owns bootstrap's control pipe and service proof. Workers
// transfer proof retirement to Channel, retaining final reports independently.
// Close may return before a pending native query retires; it cannot certify it.
type workerControlLifetime struct {
	stream net.Conn
	proof  *Authority
	source *Channel
	report io.Closer
}

func (l *workerControlLifetime) captureChannel() *Channel {
	if l.source == nil {
		l.source, l.report = sourceReportChannel(l.stream, l.proof)
	}
	return l.source
}

func (l *workerControlLifetime) interruptReader() {
	if l.source != nil {
		// A read-only native query may still own this exact pipe.
		_ = l.stream.SetReadDeadline(time.Now())
	} else {
		_ = l.stream.Close()
	}
}

func (l *workerControlLifetime) Close() error {
	if l.source != nil {
		return errors.Join(l.source.Close(), l.report.Close())
	}
	return errors.Join(l.proof.Close(), l.stream.Close())
}
