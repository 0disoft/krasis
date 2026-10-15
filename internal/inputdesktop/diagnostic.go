package inputdesktop

// DesktopClass describes a fixed, already-bound native target. It cannot select
// a desktop or supply readiness/authorization. No arbitrary OS name is exposed.
type DesktopClass uint8

const (
	DesktopUnknown DesktopClass = iota
	DesktopDefault
	DesktopWinlogon
)

// DiagnosticDesktop is read only on the owning thread, inside a successful Do
// callback. Its caller must not treat a past observation as current activity.
func (scope *Scope) DiagnosticDesktop() DesktopClass {
	if scope == nil || scope.closed.Load() || scope.root == nil || scope.root.Err() != nil || scope.owner == 0 || scope.thread == nil || scope.thread() != scope.owner {
		return DesktopUnknown
	}
	return diagnosticDesktop(scope.desktop)
}

func diagnosticDesktop(b binding) DesktopClass {
	if value, ok := b.(interface{ diagnosticDesktop() DesktopClass }); ok {
		if desktop := value.diagnosticDesktop(); desktop == DesktopDefault || desktop == DesktopWinlogon {
			return desktop
		}
	}
	return DesktopUnknown
}

func (b *interactiveBinding) diagnosticDesktop() DesktopClass {
	if b == nil || b.fenced {
		return DesktopUnknown
	}
	return diagnosticDesktop(b.current)
}
