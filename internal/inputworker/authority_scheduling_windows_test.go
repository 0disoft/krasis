//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestAuthoritySchedulingPreservesFreshProofAndBoundedPolicy(t *testing.T) {
	get := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetThreadPriority")
	for _, tc := range []struct {
		kind       authorityQueryKind
		activation bool
		want       int32
	}{
		{inputAuthorityQuery, false, 0},
		{inputAuthorityQuery, true, 1},
		{sourceInputAuthorityQuery, false, 1},
		{sourcePresenceAuthorityQuery, false, 1},
		{sourceAdvisoryAuthorityQuery, false, 0},
		{sourceWritePresenceAuthorityQuery, false, 0},
	} {
		server, pipe := net.Pipe()
		owner := clientChannel(pipe, authorityService)
		observed := make(chan int32, 2)
		failure := errors.New("original fresh proof failure")
		check := func() error {
			p, _, _ := get.Call(uintptr(windows.CurrentThread()))
			observed <- int32(p)
			return failure
		}
		owner.authority.checkInput, owner.authority.checkPresent = check, check
		ctx := context.Background()
		if tc.activation {
			ctx = context.WithValue(ctx, sourceCommandAuthorityKey{}, true)
		}
		for range 2 {
			if err := owner.queryAuthority(ctx, false, tc.kind); err == nil {
				t.Fatal("priority hint accepted a rejected fresh proof", tc)
			}
			select {
			case priority := <-observed:
				if priority != tc.want {
					t.Fatal("wrong query priority", tc, priority)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("fresh query did not run", tc)
			}
		}
		if owner.Close() != nil {
			t.Fatal("query ownership leaked", tc)
		}
		server.Close()
	}
}
