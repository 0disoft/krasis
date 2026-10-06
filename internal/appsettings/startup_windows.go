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
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer key.Close()
	value, _, err := key.GetStringValue("Krasis")
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return value == StartupCommand(base), nil
}
func SetStartup(base string, enabled bool) error {
	if !filepath.IsAbs(base) || strings.ContainsAny(base, "\"\r\n") {
		return errors.New("invalid startup root")
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
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
	actual, err := StartupEnabled(base)
	if err != nil {
		return err
	}
	if actual != enabled {
		return errors.New("startup verification failed")
	}
	return nil
}
