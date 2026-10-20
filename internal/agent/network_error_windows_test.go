//go:build windows

package agent

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/0disoft/krasis/internal/failurecode"
	"golang.org/x/sys/windows"
)

func TestWrappedWinsockFailuresRemainDistinctAtAcquireAndSessionEnd(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{windows.WSAEACCES, "network_access_denied"},
		{windows.WSAETIMEDOUT, "network_timeout"},
		{windows.WSAECONNREFUSED, "network_refused"},
		{windows.WSAECONNRESET, "network_reset"},
		{windows.WSAENETRESET, "network_reset"},
		{windows.WSAECONNABORTED, "network_aborted"},
		{windows.WSAENETUNREACH, "network_unreachable"},
		{windows.WSAEHOSTUNREACH, "network_unreachable"},
	} {
		err := &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connectex", Err: tc.err}}
		d := describeAcquisitionFailure(err)
		if d.Code != tc.want || failurecode.ReconnectCode("acquire", d.Component, d.Phase, d.Code) != "reconnect_acquire_"+tc.want {
			t.Fatalf("%v => %+v", tc.err, d)
		}
		v := describeSessionTermination(errors.Join(errors.New("private endpoint"), err), true)
		if v.NetworkError != tc.want || !v.NativeKnown {
			t.Fatalf("session lost Winsock evidence: %+v", v)
		}
	}
	if got := describeSessionTermination(&os.SyscallError{Syscall: "pipe", Err: windows.ERROR_ACCESS_DENIED}, true); got.NetworkError != "not_observed" {
		t.Fatal("pipe failure labelled network", got)
	}
	if d := describeAcquisitionFailure(context.Canceled); d.Code != "network_canceled" {
		t.Fatal(d)
	}
}
