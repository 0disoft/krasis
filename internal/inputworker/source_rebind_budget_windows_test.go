//go:build windows

package inputworker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sys/windows"
)

func TestSourceRebindSurvivesDelayedServiceJoinWithoutReplayingLease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		endpointReady := started.Add(3 * time.Second) // Beyond the existing 2s open.
		calls := 0
		var observed []string
		want := &Channel{}
		got, err := connectSourceRebind(context.Background(), true, func(ctx context.Context) (*Channel, error) {
			calls++
			attempt, cancel := context.WithTimeout(ctx, serviceConnectTimeout)
			defer cancel()
			timer := time.NewTimer(time.Until(endpointReady))
			defer timer.Stop()
			select {
			case <-attempt.Done():
				return nil, connectionFailed("pipe_open", attempt.Err())
			case <-timer.C:
				return want, nil // Only opening/authentication; no lease or input.
			}
		}, func(code string) { observed = append(observed, code) })
		if err != nil || got != want || calls != 2 || time.Since(started) != 3*time.Second {
			t.Fatal("clean retirement could not rebind after delayed service join", err, calls, time.Since(started))
		}
		if fmt.Sprint(observed) != "[installed_source_rebind_wait installed_source_rebind_ready]" {
			t.Fatal("missing bounded rebind evidence", observed)
		}
	})
}

func TestSourceRebindRetriesOnlyKnownOpenFailuresAfterCleanRetirement(t *testing.T) {
	for _, retired := range []bool{false, true} {
		for _, failure := range []error{
			fmt.Errorf("wrapped: %w", connectionFailed("pipe_open", windows.ERROR_PIPE_BUSY)),
			connectionFailed("pipe_open", context.DeadlineExceeded),
			connectionFailed("pipe_open", windows.ERROR_FILE_NOT_FOUND),
			connectionFailed("pipe_open", windows.ERROR_PIPE_BUSY),
			connectionFailed("pipe_open", windows.ERROR_ACCESS_DENIED),
			connectionFailed("service_status", context.DeadlineExceeded),
			connectionFailed("pipe_open", errors.Join(context.DeadlineExceeded, ErrRecovery)),
			errors.Join(connectionFailed("pipe_open", context.DeadlineExceeded), ErrRecovery),
			ErrAuthority, context.Canceled,
		} {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				want := &Channel{}
				got, err := connectSourceRebind(context.Background(), retired, func(context.Context) (*Channel, error) {
					calls++
					if calls == 1 {
						return nil, failure
					}
					return want, nil
				}, nil)
				if retired && sourceRebindOpenTransient(failure) {
					if calls != 2 || err != nil || got != want {
						t.Fatal("safe new open was not retried", calls, err)
					}
				} else if calls != 1 || err != failure || got != nil {
					t.Fatal("initial open, authority denial or cleanup failure retried", calls, err)
				}
			})
		}
	}
}

func TestSourceRebindRejectsInvalidOrUncertainReturnedChannel(t *testing.T) {
	for _, closeFailure := range []error{nil, ErrAuthority} {
		server, pipe := net.Pipe()
		channel := clientChannel(pipe, authorityService)
		channel.authority.closeHandles = func() error { return closeFailure }
		calls := 0
		cause := connectionFailed("pipe_open", context.DeadlineExceeded)
		got, err := connectSourceRebind(context.Background(), true, func(context.Context) (*Channel, error) {
			calls++
			return channel, cause
		}, nil)
		server.Close()
		if got != nil || calls != 1 || !errors.Is(err, cause) || errors.Is(err, ErrRecovery) != (closeFailure != nil) || !channel.closing {
			t.Fatal("partial connection leaked or cleanup uncertainty was retried", calls, err)
		}
	}
	if got, err := connectSourceRebind(context.Background(), true, func(context.Context) (*Channel, error) { return nil, nil }, nil); got != nil || err != ErrProtocol {
		t.Fatal("empty successful connection accepted", got, err)
	}
}

func TestSourceRebindUsesOneDeadlineAndHonorsEarlierCancellation(t *testing.T) {
	for _, budget := range []time.Duration{30 * time.Millisecond, sourceRebindTimeout, 2 * sourceRebindTimeout} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			defer cancel()
			started, calls := time.Now(), 0
			cause := connectionFailed("pipe_open", windows.ERROR_FILE_NOT_FOUND)
			got, err := connectSourceRebind(ctx, true, func(context.Context) (*Channel, error) {
				calls++
				return nil, cause
			}, nil)
			if got != nil || !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, cause) || time.Since(started) != min(budget, sourceRebindTimeout) || calls > 54 {
				t.Fatal("rebind renewed its budget, lost cause or spun", calls, time.Since(started), err)
			}
		})
	}
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		_, err := connectSourceRebind(ctx, true, func(context.Context) (*Channel, error) {
			calls++
			cancel()
			return nil, connectionFailed("pipe_open", windows.ERROR_PIPE_BUSY)
		}, nil)
		if calls != 1 || !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled source attempted another open", calls, err)
		}
	})
}

func TestSourceRebindClosesLateSuccessfulChannelBeforeDenyingIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		channel := clientChannel(pipe, authorityService)
		defer channel.Close()
		ctx, cancel := context.WithCancel(context.Background())
		got, err := connectSourceRebind(ctx, true, func(context.Context) (*Channel, error) {
			cancel()
			return channel, nil
		}, nil)
		if got != nil || !errors.Is(err, context.Canceled) || !channel.closing {
			t.Fatal("late open leaked handles or admitted capture", err)
		}
	})
}
