//go:build windows

package inputdesktop

import (
	"context"
	"errors"
	"testing"
)

func TestDesktopNameProofIsHandleBoundWhileActivityRemainsFresh(t *testing.T) {
	previousName, previousInput := readDesktopName, readDesktopInput
	t.Cleanup(func() { readDesktopName, readDesktopInput = previousName, previousInput })
	names, activity := 0, 0
	receiving := true
	readDesktopName = func(handle uintptr) (string, error) {
		names++
		if handle != 10 {
			t.Fatal("wrong name validation handle")
		}
		return "Default", nil
	}
	readDesktopInput = func(handle uintptr) (bool, error) {
		activity++
		if handle != 10 {
			t.Fatal("unverified replacement queried")
		}
		return receiving, nil
	}
	d := &desktopBinding{owned: 10}
	if !errors.Is(d.active(), ErrInactive) || names != 0 || activity != 0 {
		t.Fatal("unvalidated binding admitted work")
	}
	if d.verifyName() != nil {
		t.Fatal("valid Default denied")
	}
	for range 10 {
		if d.active() != nil {
			t.Fatal("active desktop refused")
		}
	}
	if names != 1 || activity != 10 {
		t.Fatal("name repeatedly queried or activity cached", names, activity)
	}
	receiving = false
	if !errors.Is(d.active(), ErrInactive) || activity != 11 {
		t.Fatal("desktop switch retained input authority")
	}
	readDesktopInput = func(uintptr) (bool, error) { return false, errors.New("query failed") }
	if !errors.Is(d.active(), ErrInactive) {
		t.Fatal("failed activity query admitted work")
	}
	d.owned = 20
	if !errors.Is(d.active(), ErrInactive) || d.verifiedHandle != 10 {
		t.Fatal("replacement inherited old name proof")
	}
	d.owned = 0
	if !errors.Is(d.active(), ErrClosed) {
		t.Fatal("closed desktop retained proof")
	}
}

func TestDesktopNameVerificationRejectsOtherNamesAndReadFailures(t *testing.T) {
	previous := readDesktopName
	t.Cleanup(func() { readDesktopName = previous })
	for _, name := range []string{"", "default", "Winlogon"} {
		readDesktopName = func(uintptr) (string, error) { return name, nil }
		d := &desktopBinding{owned: 10}
		if !errors.Is(d.verifyName(), ErrInactive) || d.verifiedHandle != 0 {
			t.Fatal("non-Default name accepted")
		}
	}
	readDesktopName = func(uintptr) (string, error) { return "Default", errors.New("denied") }
	if d := (&desktopBinding{owned: 10}); !errors.Is(d.verifyName(), ErrInactive) || d.verifiedHandle != 0 {
		t.Fatal("failed name read accepted")
	}
	if !errors.Is((*desktopBinding)(nil).verifyName(), ErrClosed) || !errors.Is((&desktopBinding{}).verifyName(), ErrClosed) {
		t.Fatal("unknown handle acquired proof")
	}
	d := &desktopBinding{owned: 10, verifiedHandle: 10}
	if !errors.Is(d.verifyName(), ErrInactive) || d.verifiedHandle != 0 {
		t.Fatal("failed revalidation retained previous proof")
	}
}

// Read-only native validation on the owned thread: no input is injected.
// This compares the removed repeated name read, not end-to-end KVM latency.
func BenchmarkDesktopScopeValidation(b *testing.B) {
	for _, repeatName := range []bool{true, false} {
		name := "handle-name-once"
		if repeatName {
			name = "name-on-every-operation"
		}
		b.Run(name, func(b *testing.B) {
			err := Run(context.Background(), func(scope *Scope) error {
				d := scope.desktop.(*desktopBinding)
				b.ReportAllocs()
				for b.Loop() {
					if repeatName {
						name, err := readDesktopName(d.owned)
						if err != nil || name != "Default" {
							return ErrInactive
						}
					}
					if err := scope.Do(context.Background(), func() error { return nil }); err != nil {
						return err
					}
				}
				return nil
			})
			if errors.Is(err, ErrUnavailable) && !errors.Is(err, ErrCleanup) {
				b.Skip("no accessible active Default desktop")
			}
			if err != nil {
				b.Fatal(err)
			}
		})
	}
}
