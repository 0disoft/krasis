package inputworker

import (
	"errors"
	"io"
	"net"
	"os"
	"testing"
)

type submissionFallbackConn struct {
	net.Conn
	n, calls int
	err      error
}

func (c *submissionFallbackConn) Write([]byte) (int, error) { c.calls++; return c.n, c.err }

func TestSourceSubmissionFallbackNeverRetriesUncertainWrites(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int
		err  error
		want error
	}{
		{"complete", 3, nil, nil},
		{"short", 1, nil, io.ErrShortWrite},
		{"attempted timeout", 0, os.ErrDeadlineExceeded, os.ErrDeadlineExceeded},
		{"partial timeout", 1, os.ErrDeadlineExceeded, os.ErrDeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &submissionFallbackConn{n: tc.n, err: tc.err}
			submitted := uint64(999)
			err := writeSourceBytes(c, []byte{1, 2, 3}, &submitted)
			if c.calls != 1 || submitted != 0 || !errors.Is(err, tc.want) {
				t.Fatal(c.calls, submitted, err)
			}
			if err != nil {
				var failure *sourceIOFailure
				if !errors.As(err, &failure) || failure.unsubmitted {
					t.Fatal("attempted write gained replay proof", err)
				}
			}
		})
	}
}
