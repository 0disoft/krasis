package inputworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestLoginOwnerRecoversHeldInputAfterUncooperativeNativeChild(t *testing.T) {
	process, client := startPortableFixture(t, "hang")
	recovery := &recordingInput{}
	native := newIsolatedInput(client, process, exitCheckedRelease{recovery, process.Done()})
	started := time.Now()
	report, err := runLoginInput(context.Background(), native, fakeLoginBind(loginTestDo), func(ctx context.Context, port application.InputPort) error {
		if err := port.Press(ctx, loginTestKey); err != nil {
			return err
		}
		return port.Move(ctx, 1, 0)
	})
	if err == nil || report.Recovery.Released != 1 || report.Recovery.Remaining != 0 || !native.recovering || recovery.released != 1 || recovery.pressed != 0 {
		t.Fatal("login journal did not survive native child retirement", report, err)
	}
	if time.Since(started) > 8*time.Second {
		t.Fatal("login native stall exceeded bounded recovery")
	}
}

func TestLoginOwnerDoesNotReleaseBeforeCompleteNativeRetirement(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "held"}[held], func(t *testing.T) {
			failure := errors.New("native retirement not acknowledged")
			recovery := &recordingInput{}
			client := &Client{localExchange: func(context.Context, request, func(response)) error { return nil }}
			native := &isolatedInput{Client: client, recovery: recovery, retire: func() (bool, error) { return false, failure }}
			report, err := runLoginInput(context.Background(), native, fakeLoginBind(loginTestDo), func(ctx context.Context, port application.InputPort) error {
				if held {
					return port.Press(ctx, loginTestKey)
				}
				return nil
			})
			remaining := 0
			if held {
				remaining = 1
			}
			if !errors.Is(err, ErrRecovery) || !errors.Is(err, failure) || report.Recovery.Remaining != remaining || native.recovering || recovery.released != 0 {
				t.Fatal("uncertain login input owner permitted recovery", report, err)
			}
		})
	}
}
