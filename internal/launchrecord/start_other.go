//go:build !windows

package launchrecord

func Start(string) error { return ErrInvalid }
