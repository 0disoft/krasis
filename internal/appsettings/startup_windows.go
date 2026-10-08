//go:build windows

package appsettings

import (
	"errors"
	"golang.org/x/sys/windows/registry"
	"path/filepath"
	"strings"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func StartupCommand(base string) string {
	return `"` + filepath.Join(base, "Krasis", "launch", "krasis-cli.exe") + `" relaunch --startup`
}
func StartupEnabled(base string) (bool, error) {
	return startupEnabledAt(registry.CURRENT_USER, runKey, base)
}

func startupOwned(key registry.Key, base string) (bool, error) {
	value, _, err := key.GetStringValue("Krasis")
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if value != StartupCommand(base) {
		return false, errors.New("startup registration is not owned by Krasis")
	}
	return true, nil
}

func startupEnabledAt(root registry.Key, path, base string) (bool, error) {
	key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer key.Close()
	return startupOwned(key, base)
}

// ValidateStartupRegistration is read-only. A missing value is a first install;
// a foreign or unreadable value must not be overwritten or deleted by setup.
func ValidateStartupRegistration(base string) error {
	_, err := StartupEnabled(base)
	return err
}
func SetStartup(base string, enabled bool) error {
	return setStartupAt(registry.CURRENT_USER, runKey, base, enabled)
}

func setStartupAt(root registry.Key, path, base string, enabled bool) error {
	if !filepath.IsAbs(base) || strings.ContainsAny(base, "\"\r\n") {
		return errors.New("invalid startup root")
	}
	key, _, err := registry.CreateKey(root, path, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if _, err := startupOwned(key, base); err != nil {
		return err
	}
	if enabled {
		err = key.SetStringValue("Krasis", StartupCommand(base))
	} else {
		err = key.DeleteValue("Krasis")
		if errors.Is(err, registry.ErrNotExist) {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	actual, err := startupEnabledAt(root, path, base)
	if err != nil {
		return err
	}
	if actual != enabled {
		return errors.New("startup verification failed")
	}
	return nil
}
