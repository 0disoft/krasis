//go:build !windows

package wininput

func ProtectedInputActive() bool { return false }

func InspectProtectedInput() ProtectedInputReason { return ProtectedInputClear }
