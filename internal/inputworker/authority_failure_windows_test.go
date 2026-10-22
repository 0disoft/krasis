//go:build windows

package inputworker

import (
	"errors"
	"net"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestSourceAuthorityRetainsNativeTokenQueryFailureWithoutInput(t *testing.T) {
	// Query a deliberately invalid token. This reads no identity or input and
	// demonstrates that tokenLogon no longer discards the original API error.
	_, cause := tokenLogon(0)
	var native syscall.Errno
	if !errors.As(cause, &native) || native == 0 {
		t.Fatal("native failed token query lost its error")
	}
	a := &Authority{checkInput: func() error { return authorityCheckFailed("authority-logon-token", cause) }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
	if a.check(false) != ErrAuthority {
		t.Fatal("native details escaped ordinary contract")
	}
	op, number := reportFailureDetails(a.checkSource())
	if reportOperations[op] != "authority-logon-token" || number != uint32(native) {
		t.Fatal("native source token failure lost", op, number)
	}
}

func TestSourceAuthorityRetainsBoundServiceStatusFailureWithoutInput(t *testing.T) {
	a := &Authority{checkInput: serviceProcessBinding(42, func() (uint32, windows.SERVICE_STATUS_PROCESS, error) {
		return 0, windows.SERVICE_STATUS_PROCESS{}, connectionFailed("service_status", syscall.Errno(6))
	}), checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
	op, native := reportFailureDetails(a.checkSource())
	if reportOperations[op] != "authority-service-status" || native != 6 || a.check(false) != ErrAuthority {
		t.Fatal("bound service query failed without preserving diagnostics", op, native)
	}
}

func TestSourceWritersRejectAuthorityWithoutLosingDetailsOrWriting(t *testing.T) {
	for _, present := range []bool{false, true} {
		for _, batch := range []bool{false, true} {
			server, client := net.Pipe()
			a := &Authority{checkInput: func() error {
				if !present {
					return authorityCheckFailed("authority-console-token", syscall.Errno(5))
				}
				return nil
			}, checkPresent: func() error { return authorityCheckFailed("authority-console-token", syscall.Errno(5)) }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
			w := &sourceWriter{channel: &Channel{stream: client, authority: a}, binding: Binding{1}}
			w.started.Store(true)
			var result error
			if batch {
				result = w.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)})
			} else {
				result = w.send(sourcePacket{kind: sourceAction}, nil)
			}
			op, native := reportFailureDetails(result)
			_ = server.Close()
			_ = w.channel.Close()
			if !errors.Is(result, ErrAuthority) || reportOperations[op] != "authority-console-token" || native != 5 || w.sequence != 0 {
				t.Fatal("source writer lost failed check or wrote without authority", present, batch, result, op, native)
			}
		}
	}
}
