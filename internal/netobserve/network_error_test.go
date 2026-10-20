package netobserve

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"
)

func TestSocketFailureNeverInventsPolicyEvidence(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "network_timeout"}, {context.Canceled, "network_canceled"},
		{syscall.EACCES, "network_access_denied"}, {syscall.ECONNRESET, "network_reset"},
		{syscall.ECONNREFUSED, "network_refused"}, {errors.New("SECRET firewall policy"), "network_failed"},
	} {
		a := DialAttempt{Address: "192.0.2.1:3", Phase: "tcp", Err: &net.OpError{Op: "dial", Net: "tcp", Err: tc.err}}
		v := summarizeDial(a, dialPath{}, [32]byte{}, false, "", 0)
		if !v.Valid() || v.Error != tc.want || v.Policy != "unknown" || v.NetworkProfile != "unknown" {
			t.Fatal(v)
		}
		v.Policy = "blocked"
		if v.Valid() {
			t.Fatal("unsupported policy assertion accepted")
		}
	}
}
