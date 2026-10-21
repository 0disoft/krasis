package inputworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestLoginJournalRecoveryRetiresBothNativeChildren(t *testing.T) {
	for _, mode := range []string{"release", "release-hang"} {
		t.Run(mode, func(t *testing.T) {
			process, client := startPortableFixture(t, "normal")
			releaseProcess, releaseClient := startPortableFixture(t, mode)
			input := newIsolatedInput(client, process, releaseClient)
			releaseOwner := newIsolatedInput(releaseClient, releaseProcess, nil)
			input.recoveryRetire = releaseOwner.retire
			parentChecks := 0
			forbiddenParent := func(context.Context, func() error) error {
				parentChecks++
				return ErrAuthority
			}
			done := make(chan finish, 1)
			go func() {
				report, err := runLoginInput(context.Background(), input, fakeLoginBind(forbiddenParent), func(ctx context.Context, port application.InputPort) error {
					return port.Press(ctx, loginTestKey)
				})
				done <- finish{report, err}
			}()
			select {
			case got := <-done:
				if mode == "release" {
					if got.err != nil || got.report.Recovery.Released != 1 || got.report.Recovery.Remaining != 0 {
						t.Fatal(got)
					}
				} else if !errors.Is(got.err, ErrRecovery) || got.report.Recovery.Remaining != 1 || got.report.Recovery.Released != 0 {
					t.Fatal("unacknowledged login release lost held state", got)
				}
			case <-time.After(8 * time.Second):
				t.Fatal("native login release blocked journal owner")
			}
			if parentChecks != 0 {
				t.Fatal("login journal queried native authority", parentChecks)
			}
			for _, child := range []<-chan struct{}{process.Done(), releaseProcess.Done()} {
				select {
				case <-child:
				default:
					t.Fatal("login recovery returned before exact native child exit")
				}
			}
		})
	}
}

func TestLoginJournalCannotHideUnretiredRecoveryChild(t *testing.T) {
	for _, inputRetired := range []bool{false, true} {
		closes := 0
		input := &isolatedInput{Client: &Client{}, retire: func() (bool, error) { return inputRetired, nil }, recoveryRetire: func() (bool, error) { closes++; return false, nil }}
		report, err := runLoginInput(context.Background(), input, fakeLoginBind(loginTestDo), func(context.Context, application.InputPort) error { return nil })
		if !errors.Is(err, ErrRecovery) || report.Recovery.Remaining != 0 || closes != 1 {
			t.Fatal(report, err, closes)
		}
		if !errors.Is(input.finishRecovery(), ErrRecovery) || closes != 1 {
			t.Fatal("uncertain native login retirement was retried", closes)
		}
	}
}
