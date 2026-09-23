package agent

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/transport"
)

func TestAcquireDeadlineDistinguishesNetworkFromTLS(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{{context.DeadlineExceeded, "network_timeout"}, {errors.Join(transport.ErrPeerAuthentication, context.DeadlineExceeded), "tls_timeout"}} {
		d := describeAcquisitionFailure(test.err)
		if got := productlog.ReconnectCode("acquire", d.Component, d.Phase, d.Code); got != "reconnect_acquire_"+test.want {
			t.Fatal(got)
		}
	}
}

func TestAcquireNetworkFailureSeparatesReachabilityFromAuthentication(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, "network_refused"},
		{&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ENETUNREACH}, "network_unreachable"},
		{&net.OpError{Op: "dial", Net: "tcp", Err: syscall.EHOSTUNREACH}, "network_unreachable"},
		{&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}, "network_reset"},
		{errors.New("private peer address"), "network_failed"},
	} {
		d := describeAcquisitionFailure(test.err)
		if got := productlog.ReconnectCode("acquire", d.Component, d.Phase, d.Code); got != "reconnect_acquire_"+test.want {
			t.Fatalf("error %T: got %s, want %s", test.err, got, test.want)
		}
	}
}
func TestUnauthenticatedDiagnosticsAreBoundedAndContainNoErrorText(t *testing.T) {
	source := &ListenSource{}
	var codes []string
	source.SetAuthenticationFailureObserver(func(code string) { codes = append(codes, code) })
	for i := 0; i < 100; i++ {
		source.observeAuthenticationFailure(errors.New("SECRET certificate/address"))
	}
	source.observeAuthenticationFailure(context.Canceled)
	if len(codes) != 1 || codes[0] != "tls_handshake_failed" {
		t.Fatalf("unbounded or private diagnostics: %v", codes)
	}
}
