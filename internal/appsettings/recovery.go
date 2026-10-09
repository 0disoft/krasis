package appsettings

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
)

func RecoveryPaused(base string) (bool, error) {
	var v struct {
		Paused *bool  `json:"paused"`
		Epoch  string `json:"epoch"`
	}
	err := read(base, "recovery.json", &v)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || v.Paused == nil {
		return false, errors.New("invalid recovery settings")
	}
	return *v.Paused, nil
}
func PauseRecovery(base string, paused bool) error {
	gate.Lock()
	defer gate.Unlock()
	var epoch [16]byte
	if _, err := rand.Read(epoch[:]); err != nil {
		return err
	}
	return write(base, "recovery.json", struct {
		Paused bool   `json:"paused"`
		Epoch  string `json:"epoch"`
	}{paused, hex.EncodeToString(epoch[:])})
}

// Capture an immutable stop generation; even a later explicit restart cannot
// make a stale automatic request current again. Legacy records remain readable.
func RecoveryToken(base string) (string, error) {
	var v struct {
		Paused *bool  `json:"paused"`
		Epoch  string `json:"epoch"`
	}
	err := read(base, "recovery.json", &v)
	if errors.Is(err, os.ErrNotExist) {
		return "initial", nil
	}
	if err != nil || v.Paused == nil {
		return "", errors.New("invalid recovery settings")
	}
	if *v.Paused {
		return "", errors.New("recovery paused")
	}
	hash := sha256.Sum256([]byte(v.Epoch))
	return hex.EncodeToString(hash[:]), nil
}

func RecoveryCurrent(base, token string) bool {
	settings, err := Load(base)
	if err != nil || !settings.StartAtLogin || token == "" {
		return false
	}
	current, err := RecoveryToken(base)
	return err == nil && current == token
}
