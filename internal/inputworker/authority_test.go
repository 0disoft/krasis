package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestZeroAuthorityNeverPermitsInput(t *testing.T) {
	for _, authority := range []*Authority{nil, {}} {
		if authority.check(false) != ErrAuthority || authority.check(true) != ErrAuthority {
			t.Fatal("zero authority permitted input")
		}
		server, client := net.Pipe()
		_, err := Serve(context.Background(), &Channel{stream: server, authority: authority}, &recordingInput{}, testLease())
		if !errors.Is(err, ErrAuthority) {
			t.Fatal("unauthenticated stream accepted")
		}
		_ = client.Close()
	}
}

func TestAgentExitDoesNotDisableSameLogonRecovery(t *testing.T) {
	alive, sameLogon, closes := true, true, 0
	authority := &Authority{
		checkInput: func() error {
			if !alive || !sameLogon {
				return ErrAuthority
			}
			return nil
		},
		checkRecovery: func() error {
			if !sameLogon {
				return ErrAuthority
			}
			return nil
		},
		closeHandles: func() error { closes++; return nil },
	}
	if authority.check(false) != nil {
		t.Fatal("valid authority rejected")
	}
	alive = false
	if authority.check(false) != ErrAuthority || authority.check(true) != nil {
		t.Fatal("agent exit blocked recovery or allowed input")
	}
	sameLogon = false
	if authority.check(true) != ErrAuthority {
		t.Fatal("new console logon accepted old key-up")
	}
	_ = authority.Close()
	_ = authority.Close()
	if closes != 1 || authority.check(true) != ErrAuthority {
		t.Fatal("closed authority reused")
	}
}

func TestAuthorityErrorsNeverExposeNativeDetails(t *testing.T) {
	authority := &Authority{checkInput: func() error { return errors.New("synthetic private owner") }, checkRecovery: func() error { return nil }, closeHandles: func() error { return errors.New("synthetic private path") }}
	if authority.check(false) != ErrAuthority || authority.Close() != ErrAuthority {
		t.Fatal("native authorization error leaked")
	}
}

func TestServerProofCannotAuthorizeAgentInput(t *testing.T) {
	authority := &Authority{kind: authorityService, checkInput: func() error { return nil }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}
	server, client := net.Pipe()
	defer client.Close()
	input := &recordingInput{}
	_, err := Serve(context.Background(), &Channel{stream: server, authority: authority}, input, testLease())
	if err != ErrAuthority || input.pressed != 0 {
		t.Fatal("service proof used as agent input authority")
	}
}
