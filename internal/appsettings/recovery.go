package appsettings

import (
	"errors"
	"os"
)

func RecoveryPaused(base string) (bool, error) {
	var v struct {
		Paused bool `json:"paused"`
	}
	err := read(base, "recovery.json", &v)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return v.Paused, err
}
func PauseRecovery(base string, paused bool) error {
	gate.Lock()
	defer gate.Unlock()
	return write(base, "recovery.json", struct {
		Paused bool `json:"paused"`
	}{paused})
}
