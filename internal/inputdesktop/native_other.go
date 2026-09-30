//go:build !windows

package inputdesktop

func openDesktop() (binding, error) { return nil, ErrUnavailable }
func currentThreadID() uintptr      { return 0 }
