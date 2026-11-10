//go:build !windows && !linux

package launchrecord

func StartAtLogin(path string) error         { return Start(path) }
func StartRecovery(path, token string) error { return Start(path) }
