package residentagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
)

// ListenSource intentionally closes sockets accepted before Next installs its
// round. A goroutine launch is not a readiness barrier. Retry only that initial
// read-only TLS EOF, within the existing test context, before ANY admission or
// rejection has occurred. Never conceal an admitted handshake's failure.
func dialBeforeListenRound(ctx context.Context, dial func(context.Context) (agent.Connection, error), metrics func() agent.PreAuthMetrics) (agent.Connection, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		conn, err := dial(ctx)
		if conn != nil || !errors.Is(err, io.EOF) || metrics() != (agent.PreAuthMetrics{}) {
			return conn, err
		}
		if err := agent.Wait(ctx, time.Millisecond); err != nil {
			return nil, err
		}
	}
}

func TestPreparedTransportRetriesOnlyBeforeListenRound(t *testing.T) {
	for _, tc := range []struct {
		name    string
		metrics agent.PreAuthMetrics
	}{
		{"not_yet_installed", agent.PreAuthMetrics{}},
		{"admitted", agent.PreAuthMetrics{Admitted: 1}},
		{"busy", agent.PreAuthMetrics{RejectedBusy: 1}},
		{"invalid_source", agent.PreAuthMetrics{RejectedInvalidSource: 1}},
		{"source_capacity", agent.PreAuthMetrics{RejectedSourceCapacity: 1}},
		{"source_rate", agent.PreAuthMetrics{RejectedSourceRate: 1}},
		{"global_rate", agent.PreAuthMetrics{RejectedGlobalRate: 1}},
		{"handshake_failed", agent.PreAuthMetrics{HandshakeFailed: 1}},
		{"authenticated", agent.PreAuthMetrics{Authenticated: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			calls := 0
			startupEOF := fmt.Errorf("initial TLS: %w", io.EOF)
			laterFailure := errors.New("later handshake failure")
			_, err := dialBeforeListenRound(ctx, func(context.Context) (agent.Connection, error) {
				calls++
				if calls == 1 {
					return nil, startupEOF
				}
				return nil, laterFailure
			}, func() agent.PreAuthMetrics { return tc.metrics })
			wantCalls, wantErr := 1, startupEOF
			if tc.metrics == (agent.PreAuthMetrics{}) {
				wantCalls, wantErr = 2, laterFailure
			}
			if calls != wantCalls || err != wantErr {
				t.Fatalf("calls=%d err=%v; want %d and %v", calls, err, wantCalls, wantErr)
			}
		})
	}
}

func TestPreparedTransportStartupRetryHonorsCancellation(t *testing.T) {
	for _, cancelBefore := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if cancelBefore {
			cancel()
		}
		calls := 0
		_, err := dialBeforeListenRound(ctx, func(context.Context) (agent.Connection, error) {
			calls++
			cancel()
			return nil, io.EOF
		}, func() agent.PreAuthMetrics { return agent.PreAuthMetrics{} })
		cancel()
		wantCalls := 1
		if cancelBefore {
			wantCalls = 0
		}
		if err != context.Canceled || calls != wantCalls {
			t.Fatalf("canceled before=%v calls=%d err=%v", cancelBefore, calls, err)
		}
	}
}
