package wininput

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
)

func TestLinuxSourceCancellationPreservesIndependentFailures(t *testing.T) {
	native := errors.New("native cleanup failed")
	for _, test := range []struct {
		name          string
		err, canceled error
		want          bool
	}{
		{"live closed owner", net.ErrClosed, nil, false},
		{"live EOF", io.EOF, nil, false},
		{"cancel only", context.Canceled, context.Canceled, true},
		{"deadline only", context.DeadlineExceeded, context.DeadlineExceeded, true},
		{"closed owner", net.ErrClosed, context.Canceled, true},
		{"wrapped closed owner", &net.OpError{Op: "set", Net: "unix", Err: net.ErrClosed}, context.Canceled, true},
		{"joined cancellation", errors.Join(context.Canceled, net.ErrClosed), context.Canceled, true},
		{"closed XGB reply", io.EOF, context.Canceled, true},
		{"wrapped XGB reply", fmt.Errorf("X11 query: %w", io.EOF), context.Canceled, true},
		{"in-flight XGB cancellation", errors.Join(context.Canceled, io.EOF, net.ErrClosed), context.Canceled, true},
		{"truncated XGB reply", io.ErrUnexpectedEOF, context.Canceled, false},
		{"native failure", native, context.Canceled, false},
		{"joined native failure", errors.Join(context.Canceled, net.ErrClosed, native), context.Canceled, false},
		{"joined EOF and native failure", errors.Join(context.Canceled, io.EOF, native), context.Canceled, false},
		{"wrapped joined failure", fmt.Errorf("cleanup: %w", errors.Join(net.ErrClosed, native)), context.Canceled, false},
		{"cleanup timeout", errors.Join(net.ErrClosed, context.DeadlineExceeded), context.Canceled, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := linuxSourceCancellationOnly(test.err, test.canceled); got != test.want {
				t.Fatalf("cancellation-only=%v, want %v: %v", got, test.want, test.err)
			}
		})
	}
}
