//go:build !windows && !linux

package launchrecord

func Start(string) error { return ErrInvalid }
