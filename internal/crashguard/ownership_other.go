//go:build !windows

package crashguard

func CheckPortableRecovery() error { return nil }
