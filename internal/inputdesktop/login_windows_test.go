//go:build windows

package inputdesktop

import (
	"context"
	"errors"
	"testing"
)

func TestDesktopProofNeverFallsBackBetweenDefaultAndWinlogon(t *testing.T) {
	previousName, previousInput := readDesktopName, readDesktopInput
	t.Cleanup(func() { readDesktopName, readDesktopInput = previousName, previousInput })
	var receiving bool = true
	readDesktopInput = func(uintptr) (bool, error) { return receiving, nil }
	for _, target := range []desktopTarget{defaultTarget, winlogonTarget, 255} {
		for _, name := range []string{"Default", "Winlogon", "ScreenSaver", ""} {
			readDesktopName = func(uintptr) (string, error) { return name, nil }
			d := &desktopBinding{target: target, owned: 10}
			want := target == defaultTarget && name == "Default" || target == winlogonTarget && name == "Winlogon"
			err := d.verifyName()
			if want {
				if err != nil || d.active() != nil {
					t.Fatal("fixed target rejected", err)
				}
				receiving = false
				if !errors.Is(d.active(), ErrInactive) {
					t.Fatal("inactive desktop allowed input")
				}
				receiving = true
			} else if err == nil || d.active() == nil || d.verifiedHandle != 0 {
				t.Fatal("wrong desktop granted input proof", target, name)
			}
		}
	}
}

func TestWinlogonScopeRequiresAuthorityBeforeCallback(t *testing.T) {
	called := false
	err := RunWinlogon(context.Background(), func() error { return errors.New("untrusted channel") }, func(*Scope) error { called = true; return nil })
	if !errors.Is(err, ErrUnavailable) || called {
		t.Fatal("unauthenticated scope entered", err)
	}
	if err := RunWinlogon(nil, func() error { return nil }, func(*Scope) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := RunWinlogon(context.Background(), nil, func(*Scope) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestOrdinaryProcessCannotEnterWinlogonEvenWithClaimedAuthority(t *testing.T) {
	if privilegedLoginToken() {
		t.Skip("ordinary-process denial needs a non-service test runner")
	}
	called := false
	err := RunWinlogon(context.Background(), func() error { return nil }, func(*Scope) error { called = true; return nil })
	if !errors.Is(err, ErrUnavailable) || called {
		t.Fatal("claimed service authority bypassed actual token check", err)
	}
}
