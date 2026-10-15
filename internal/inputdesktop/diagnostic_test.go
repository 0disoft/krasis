package inputdesktop

import (
	"context"
	"testing"
)

type diagnosticBinding struct{ target DesktopClass }

func (*diagnosticBinding) active() error                     { return nil }
func (*diagnosticBinding) close() error                      { return nil }
func (b *diagnosticBinding) diagnosticDesktop() DesktopClass { return b.target }

func TestDesktopDiagnosticUsesExactBoundOwnerWithoutSelectingOrOpening(t *testing.T) {
	b := &interactiveBinding{current: &diagnosticBinding{target: DesktopDefault}}
	s := &Scope{root: context.Background(), desktop: b, owner: 1, thread: func() uintptr { return 1 }}
	if s.DiagnosticDesktop() != DesktopDefault {
		t.Fatal("bound Default lost")
	}
	b.current = &diagnosticBinding{target: DesktopWinlogon}
	if s.DiagnosticDesktop() != DesktopWinlogon {
		t.Fatal("bound Winlogon lost")
	}
	b.fenced = true
	if s.DiagnosticDesktop() != DesktopUnknown {
		t.Fatal("fenced target reported")
	}
	b.fenced = false
	b.current = nil
	if s.DiagnosticDesktop() != DesktopUnknown {
		t.Fatal("missing target inferred")
	}
	b.current = &diagnosticBinding{target: 255}
	if s.DiagnosticDesktop() != DesktopUnknown {
		t.Fatal("unbounded target reported")
	}
	b.current = &diagnosticBinding{target: DesktopWinlogon}
	s.thread = func() uintptr { return 2 }
	if s.DiagnosticDesktop() != DesktopUnknown {
		t.Fatal("read target from another thread")
	}
	s.thread = func() uintptr { return 1 }
	s.closed.Store(true)
	if s.DiagnosticDesktop() != DesktopUnknown {
		t.Fatal("closed owner reported")
	}
}
