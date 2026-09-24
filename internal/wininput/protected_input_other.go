//go:build !windows

package wininput

func ProtectedInputActive() bool { return false }
