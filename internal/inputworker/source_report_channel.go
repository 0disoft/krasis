package inputworker

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
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
