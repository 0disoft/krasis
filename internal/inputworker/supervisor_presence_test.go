package inputworker

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestSupervisorConsumesRecoveryWhileFullAuthorityIsBlocked(t *testing.T) {
	for _, remaining := range []int{0, 1} {
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		var presence atomic.Int32
		a := &Authority{
			checkInput: func() error { close(entered); <-release; return nil },
			checkPresent: func() error {
				presence.Add(1)
				return nil
			},
			checkRecovery: func() error { return nil },
			closeHandles:  func() error { return nil },
		}
		fullDone := make(chan error, 1)
		go func() { fullDone <- a.check(false) }()
		<-entered
		server, client := net.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan workerReportResult, 1)
		go func() {
			report, err := supervise(ctx, client, a.checkHeartbeat, func() bool { return false }, time.Now().Add(time.Hour))
			result <- workerReportResult{report, err}
		}()
		peerDone := make(chan error, 1)
		go func() {
			var stop [1]byte
			_, err := server.Read(stop[:])
			if err == nil && stop[0] != 'S' {
				err = ErrProtocol
			}
			if err == nil {
				err = writeReport(server, Report{Completed: 1, Recovery: application.ReleaseReport{Attempted: 1, Remaining: remaining}}, nil)
			}
			peerDone <- err
		}()
		limit := time.Now().Add(time.Second)
		for presence.Load() == 0 && time.Now().Before(limit) {
			time.Sleep(time.Millisecond)
		}
		cancel()
		select {
		case got := <-result:
			if got.report.Completed != 1 || got.report.Recovery.Remaining != remaining || errors.Is(got.err, ErrRecovery) != (remaining != 0) {
				t.Error("native authority query lost or fabricated recovery evidence", got)
			}
		case <-time.After(time.Second):
			t.Error("supervision stalled behind a full native authority query")
		}
		_ = server.Close()
		_ = client.Close()
		once.Do(func() { close(release) })
		if err := <-fullDone; err != nil {
			t.Error(err)
		}
		if err := <-peerDone; err != nil || presence.Load() == 0 {
			t.Error("supervisor did not exchange the exact worker receipt", err)
		}
		_ = a.Close()
	}
}
