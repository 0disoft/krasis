//go:build !windows

package launchrecord

func UIProcessMatches(string, uint32) bool { return false }
