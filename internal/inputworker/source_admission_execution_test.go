package inputworker

import (
	"context"
	"net"
	"testing"
	"testing/synctest"
	"time"
)

func TestSourceAdmissionDefersSlowProofButRejectsRevocationAtExecution(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var owners [2]*Channel
		full, presence, applied := 0, 0, 0
		revoked := false
		for i := range owners {
			server, pipe := net.Pipe()
			defer server.Close()
			owners[i] = clientChannel(pipe, authorityService)
			defer owners[i].Close()
			owners[i].authority.checkPresent = func() error { presence++; return nil }
			owners[i].authority.checkInput = func() error {
				full++
				time.Sleep(400 * time.Millisecond)
				if revoked {
					return ErrAuthority
				}
				return nil
			}
		}
		started := time.Now()
		if err := authorizeSourceRead(context.Background(), owners[0], owners[1], sourceAdmit); err != nil || full != 0 || presence != 2 || time.Since(started) != 0 {
			t.Fatal("reader repeated execution RPCs", err, full, presence)
		}
		authorize := sourceCaptureAuthorization(context.Background(), owners[0], owners[1])
		desktop := func(_ context.Context, work func() error) error {
			if err := authorize(); err != nil {
				return err
			}
			return work()
		}
		timing, err := executeSourceCommand(context.Background(), sourceAdmit, desktop, func() error { applied++; return nil })
		if err != nil || applied != 1 || full != 2 || time.Since(started) != 800*time.Millisecond || timing != ([3]byte{}) {
			t.Fatal("execution lost fresh proofs or changed reply shape", err, full, applied, timing)
		}
		revoked = true // A prior successful read/proof must not authorize another command.
		_, err = executeSourceCommand(context.Background(), sourceAdmit, desktop, func() error { applied++; return nil })
		if err == nil || applied != 1 || full != 3 {
			t.Fatal("revoked owner reached admission work", err, full, applied)
		}
	})
}
