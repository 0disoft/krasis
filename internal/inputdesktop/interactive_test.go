package inputdesktop

import (
	"errors"
	"testing"
)

type switchingDesktop struct {
	inactive bool
	closeErr error
	closes   int
}

func (d *switchingDesktop) active() error {
	if d.inactive {
		return ErrInactive
	}
	return nil
}
func (d *switchingDesktop) close() error { d.closes++; return d.closeErr }

func TestInteractiveDesktopRetiresBeforeSwitchAndChecksAuthority(t *testing.T) {
	old := &switchingDesktop{inactive: true}
	next := &switchingDesktop{}
	checks, opens := 0, 0
	b := &interactiveBinding{current: old, authorize: func() error { checks++; return nil }, open: func() (binding, error) {
		opens++
		if old.closes != 1 {
			t.Fatal("new desktop before old retirement")
		}
		return next, nil
	}}
	if err := b.active(); err != nil || b.current != next || checks != 2 || opens != 1 {
		t.Fatalf("switch: %v %d %d", err, checks, opens)
	}
	if err := b.active(); err != nil || opens != 1 {
		t.Fatal("stable desktop reopened", err)
	}
	if err := b.close(); err != nil || next.closes != 1 {
		t.Fatal("cleanup", err)
	}
}

func TestInteractiveDesktopNeverEscapesFailedCleanupOrRevocation(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		old := &switchingDesktop{inactive: true, closeErr: errors.New("native close failed")}
		b := &interactiveBinding{current: old, authorize: func() error {
			if revoke {
				return ErrUnavailable
			}
			return nil
		}, open: func() (binding, error) { t.Fatal("opened after cleanup/authority failure"); return nil, nil }}
		if b.active() == nil {
			t.Fatal("failure hidden")
		}
		if !revoke && (!errors.Is(b.active(), ErrCleanup) || !errors.Is(b.close(), ErrCleanup) || old.closes != 1) {
			t.Fatal("failed cleanup retried")
		}
		if revoke && old.closes != 0 {
			t.Fatal("revoked request changed desktop")
		}
	}
}

func TestInteractiveOpenFailureRetainsNoStaleBinding(t *testing.T) {
	old := &switchingDesktop{inactive: true}
	b := &interactiveBinding{current: old, authorize: func() error { return nil }, open: func() (binding, error) { return nil, ErrUnavailable }}
	if !errors.Is(b.active(), ErrUnavailable) || b.current != nil || b.close() != nil || old.closes != 1 {
		t.Fatal("unavailable desktop retained or cleanup mislabeled")
	}
}
