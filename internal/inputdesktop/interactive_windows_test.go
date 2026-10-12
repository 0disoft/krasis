//go:build windows

package inputdesktop

import (
	"context"
	"errors"
	"testing"
)

func TestInteractiveDesktopRefusesOrdinaryCallerEvenWithApprovalCallback(t *testing.T) {
	if privilegedLoginToken() {
		t.Skip("this test requires an ordinary test process")
	}
	err := RunInteractive(context.Background(), func() error { return nil }, func(*Scope) error { t.Fatal("ordinary caller received interactive scope"); return nil })
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal("unprivileged callback granted input desktop", err)
	}
}

func TestPresentationDesktopRefusesOrdinaryCaller(t *testing.T) {
	if privilegedLoginToken() {
		t.Skip("this test requires an ordinary test process")
	}
	err := RunPresentation(context.Background(), func() error { return nil }, func(*Scope) error {
		t.Fatal("ordinary caller received presentation desktop")
		return nil
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal("unprivileged presentation granted", err)
	}
}
