package agent

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/0disoft/krasis/internal/netobserve"
	"github.com/0disoft/krasis/internal/peer"
)

// SessionTermination describes detection, not the inferred time/root cause of
// an OS failure. Trigger is captured before joins can add secondary errors.
// One fixed component result follows after all owners have joined.
type SessionTermination struct {
	ObservedAt   time.Time
	Trigger      bool
	Component    string
	Failure      FailureDescriptor
	Canceled     bool
	Timeout      bool
	Closed       bool
	NativeError  uint32
	NativeKnown  bool
	NetworkError string
}

func describeSessionTermination(err error, trigger bool) SessionTermination {
	v := SessionTermination{ObservedAt: time.Now().UTC(), Trigger: trigger, Component: SessionFailureComponent(err), Failure: DescribeSessionFailure(err)}
	v.NetworkError = "not_observed"
	var socket *net.OpError
	if errors.As(err, &socket) && (socket.Net == "tcp" || socket.Net == "tcp4" || socket.Net == "tcp6") {
		v.NetworkError = netobserve.NetworkError(socket)
	}
	// Only the independent watchdog cancels with these bare sentinels;
	// component-returned failures retain their own typed component wrapper.
	if err == peer.ErrPeerLivenessExpired || err == ErrLeaseHandshakeExpired {
		v.Component = "lease-watchdog"
	}
	v.Canceled = errors.Is(err, context.Canceled)
	var timeout net.Error
	v.Timeout = errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout()
	v.Closed = errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe)
	var native syscall.Errno
	if errors.As(err, &native) && uint64(native) <= uint64(^uint32(0)) {
		v.NativeError, v.NativeKnown = uint32(native), true
	}
	return v
}
