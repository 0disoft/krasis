//go:build windows

package inputworker

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestServiceAuthenticatesDuringPreparationButJoinsBeforeAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var slot serviceInputPreparation
		release := make(chan struct{})
		child := &preparedConsole{}
		builds := 0
		build := func(context.Context) (*preparedConsole, error) {
			builds++
			<-release
			return child, nil
		}
		if !slot.start(context.Background(), build) || slot.start(context.Background(), build) {
			t.Fatal("pending builder was replaced or duplicated")
		}
		slot.reenter(context.Background(), Lease{ExpiresAt: time.Now().Add(time.Minute)}, nil, build)
		synctest.Wait()
		channel := &Channel{}
		got, err := slot.accept(func() (*Channel, error) {
			// This callback is the actual service accept/authentication path.
			// The old end-of-lease join prevented it from running at all.
			if slot.next != nil || slot.warming == nil || builds != 1 {
				t.Fatal("authentication did not overlap the sole unbound builder")
			}
			go func() {
				time.Sleep(120 * time.Millisecond)
				close(release)
			}()
			return channel, nil
		})
		if err != nil || got != channel || slot.warming != nil || slot.next != child || builds != 1 {
			t.Fatal("admission lost the single prepared child", err)
		}
		if !channel.preparationWaitMeasured || channel.preparationWait != 120*time.Millisecond {
			t.Fatal("remaining join wait includes authentication or lost time", channel.preparationWait)
		}
		if slot.start(context.Background(), build) {
			t.Fatal("ready candidate allowed a second builder")
		}
		slot.reenter(context.Background(), Lease{ExpiresAt: time.Now().Add(time.Minute)}, nil, build)
		if slot.warming != nil {
			t.Fatal("reentry duplicated a ready candidate")
		}
		if candidate := takeInputPreparation(&slot.next, Lease{}); candidate != child || slot.next != nil {
			t.Fatal("candidate ownership did not transfer once")
		}
		if err := child.close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestServicePreparationRecoveryFailureFencesAuthenticatedChannel(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(map[bool]string{false: "during authentication", true: "before accept"}[completed], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var slot serviceInputPreparation
				release := make(chan struct{})
				cause := errors.New("unconfirmed native retirement")
				slot.start(context.Background(), func(context.Context) (*preparedConsole, error) {
					<-release
					return nil, errors.Join(ErrRecovery, cause)
				})
				if completed {
					close(release)
					synctest.Wait()
				}
				channel := &Channel{}
				got, err := slot.accept(func() (*Channel, error) {
					if completed {
						t.Fatal("known recovery failure reached accept")
					}
					close(release)
					return channel, nil
				})
				if got != nil || !errors.Is(err, ErrRecovery) || !errors.Is(err, cause) || slot.next != nil || slot.warming != nil {
					t.Fatal("uncertain cleanup passed admission or lost its cause", err)
				}
				if !completed && !channel.closing {
					t.Fatal("authenticated channel survived recovery failure")
				}
			})
		})
	}
}

func TestServicePreparationFailedAcceptAndShutdownRetainCleanupOwnership(t *testing.T) {
	for _, failedCleanup := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "uncertain"}[failedCleanup], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var slot serviceInputPreparation
				child := &preparedConsole{}
				if failedCleanup {
					child.worker = failedConsoleCloseForTest()
				}
				slot.start(context.Background(), func(ctx context.Context) (*preparedConsole, error) {
					<-ctx.Done()
					return child, nil
				})
				if got, err := slot.accept(func() (*Channel, error) { return nil, ErrAuthority }); got != nil || !errors.Is(err, ErrAuthority) || slot.warming == nil {
					t.Fatal("failed authentication lost the pending owner", err)
				}
				err := slot.close()
				if errors.Is(err, ErrRecovery) != failedCleanup || slot.next != nil || slot.warming != nil || child.warmState.Load() != 2 {
					t.Fatal("shutdown failed to cancel/join native cleanup", err)
				}
			})
		})
	}
}

func TestServicePreparationConstructionFailureKeepsColdFallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var slot serviceInputPreparation
		release := make(chan struct{})
		slot.start(context.Background(), func(context.Context) (*preparedConsole, error) {
			<-release
			return nil, ErrAuthority
		})
		channel := &Channel{}
		got, err := slot.accept(func() (*Channel, error) {
			close(release)
			return channel, nil
		})
		if got != channel || err != nil || slot.next != nil || slot.warming != nil || channel.closing {
			t.Fatal("ordinary construction failure blocked cold fallback", err)
		}
	})
}
