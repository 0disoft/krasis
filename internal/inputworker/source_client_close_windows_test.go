//go:build windows

package inputworker

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/0disoft/krasis/internal/wininput"
)

func TestSourceClosePreservesEmergencyAcrossCancellationAndReaderJoin(t *testing.T) {
	for _, prior := range []error{nil, context.Canceled, ErrAuthority} {
		t.Run(map[error]string{nil: "success", context.Canceled: "cancellation", ErrAuthority: "command-failure"}[prior], func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			live := &sourceClient{channel: clientChannel(conn, authorityService), done: make(chan struct{})}
			defer live.channel.Close()
			// The authenticated reader has received an emergency retirement;
			// let its publication race cancellation and cleanup on the caller.
			publish := make(chan struct{})
			go func() {
				<-publish
				live.err = wininput.ErrEmergencyRelease
				live.retired.Store(true)
				close(live.done)
			}()
			finished := make(chan error, 1)
			go func() { finished <- live.closeResult(prior) }()
			var data [1]byte
			if _, err := server.Read(data[:]); !errors.Is(err, io.EOF) {
				t.Error("cleanup did not close its stream", err)
			}
			select {
			case err := <-finished:
				t.Error("result published before the reader joined", err)
				finished <- err
			default:
			}
			close(publish)
			err := <-finished
			if !errors.Is(err, wininput.ErrEmergencyRelease) || prior != nil && !errors.Is(err, prior) || CleanSourceRetirement(err) {
				t.Fatal("emergency lost or became a restartable retirement", err)
			}
		})
	}
}

func TestSourceCloseDoesNotTurnItsOwnReaderEOFIntoARecoveryFailure(t *testing.T) {
	for _, retired := range []bool{false, true} {
		server, conn := net.Pipe()
		live := &sourceClient{channel: clientChannel(conn, authorityService), done: make(chan struct{})}
		live.retired.Store(retired)
		if !retired {
			live.err = io.EOF // Closing our own stream is not new failure evidence.
		}
		close(live.done)
		if err := live.closeResult(context.Canceled); err != context.Canceled {
			t.Error("clean cancellation acquired a manufactured failure", err)
		}
		server.Close()
	}
}
