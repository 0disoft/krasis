//go:build !windows

package appsettings

func SetRecoveryTask(string, bool) error { return nil }
