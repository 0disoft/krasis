//go:build !windows

package appsettings

func RecoveryTaskEnabled(string) (bool, error) { return false, nil }

func ValidateRecoveryTaskRegistration(string) error { return nil }

func SetRecoveryTask(string, bool) error { return nil }
