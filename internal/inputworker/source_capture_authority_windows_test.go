//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/wininput"
)

func TestSourceCommandDesktopCannotOutliveNativeAuthorityBudget(t *testing.T) {
	for _, stalled := range []int{0, 1} {
		synctest.Test(t, func(t *testing.T) {
			owners := make([]*Channel, 2)
			for i := range owners {
				server, pipe := net.Pipe()
				defer server.Close()
				owners[i] = clientChannel(pipe, authorityService)
			}
			release := make(chan struct{})
			owners[stalled].authority.checkInput = func() error {
				<-release
				return ErrAuthority // Never enter a real desktop or inject input.
			}
			timer := time.AfterFunc(2*time.Second, func() { close(release) })
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			hook, err := wininput.NewProtectedSourceHook(sourceCaptureAuthorization(ctx, owners[0], owners[1]))
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			err = hook.WithSourceCommandDesktop(ctx, func(context.Context) error {
				t.Error("rejected authority entered desktop work")
				return ErrProtocol
			})
			if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 50*time.Millisecond {
				t.Errorf("command desktop waited for uncooperative proof: owner=%d elapsed=%s err=%v", stalled, time.Since(started), err)
			}
			if timer.Stop() {
				close(release)
			}
			synctest.Wait()
			for _, owner := range owners {
				if owner.Close() != nil {
					t.Error("native proof retirement failed")
				}
			}
		})
	}
}
