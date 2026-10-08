//go:build windows

package appinstall

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows/registry"
	"path/filepath"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Krasis`

// A receipt alone does not prove that this process can see the user's shell
// registration. Refuse removal in an unrelated/isolated registry context before
// deleting files, even when both processes report the same user SID.
func validateUninstallAt(root registry.Key, key, path string) error {
	k, err := registry.OpenKey(root, key, registry.QUERY_VALUE)
	if err != nil {
		return errors.Join(ErrInvalid, err)
	}
	defer k.Close()
	value, _, err := k.GetStringValue("InstallLocation")
	if err != nil || value != path {
		return ErrInvalid
	}
	return nil
}

func validateUninstall(path string) error {
	return validateUninstallAt(registry.CURRENT_USER, uninstallKey, path)
}

// Missing shell state is repairable. Foreign or unreadable state must be caught
// before setup stages images or replaces the receipt and launch pointers.
func validateInstallRegistrationAt(root registry.Key, key, path string) error {
	k, err := registry.OpenKey(root, key, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	return validateInstallLocation(k, path)
}

func validateInstallLocation(k registry.Key, path string) error {
	previous, _, err := k.GetStringValue("InstallLocation")
	if errors.Is(err, registry.ErrNotExist) {
		return nil // interrupted registration before InstallLocation was written
	}
	if err != nil {
		return err
	}
	if previous != path {
		return ErrInvalid
	}
	return nil
}

func registerUninstall(path string, generation int) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := validateInstallLocation(k, path); err != nil {
		return err
	}
	setup := filepath.Join(path, "krasis-setup.exe")
	for name, value := range map[string]string{"DisplayName": "Krasis", "DisplayVersion": fmt.Sprintf("g%d", generation), "Publisher": "Rodisoft", "InstallLocation": path, "UninstallString": `"` + setup + `" --remove`, "ModifyPath": `"` + setup + `" --repair`, "DisplayIcon": setup + ",0"} {
		if err = k.SetStringValue(name, value); err != nil {
			return err
		}
	}
	return k.SetDWordValue("NoRepair", 0)
}
func removeUninstall(path string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	value, _, err := k.GetStringValue("InstallLocation")
	k.Close()
	if err != nil || value != path {
		return ErrInvalid
	}
	return registry.DeleteKey(registry.CURRENT_USER, uninstallKey)
}
