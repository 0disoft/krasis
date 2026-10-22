// Package appsettings owns per-user startup and release-update preferences.
package appsettings

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

// Internal GUI entrypoint for the scheduled task; it never creates a console.
const RecoveryArgument = "--background-recovery"

type Settings struct {
	Schema       string `json:"schema"`
	StartAtLogin bool   `json:"startAtLogin"`
	AutoUpdate   bool   `json:"autoUpdate"`
}
type Resume struct {
	ManualStop  bool   `json:"manualStop,omitempty"`
	Alias       string `json:"alias"`
	Fingerprint string `json:"fingerprint"`
}

var (
	gate               sync.Mutex
	ErrInvalidResume   = errors.New("invalid resume settings")
	errInvalidDocument = errors.New("invalid settings document")
)

func Load(base string) (Settings, error) {
	return loadSettings(func(v any) error { return read(base, "settings.json", v) }, true)
}

// Initialize is an explicit normal-app startup write. Privileged readers keep
// requiring existing policy; read-only Load never creates it. No-replace
// promotion preserves a concurrent user's settings, including disabled flags.
func Initialize(base string) (Settings, error) {
	preferences, err := LockPreferences(base)
	if err != nil {
		return Settings{}, err
	}
	defer preferences.Close()
	return preferences.Initialize()
}

func initializeSettings(base string) (Settings, error) {
	gate.Lock()
	defer gate.Unlock()
	value, err := loadSettings(func(v any) error { return read(base, "settings.json", v) }, false)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return value, err
	}
	value = Settings{Schema: Schema, StartAtLogin: true, AutoUpdate: true}
	if err = writeMode(base, "settings.json", value, false); err != nil && !errors.Is(err, os.ErrExist) {
		return Settings{}, err
	}
	return loadSettings(func(v any) error { return read(base, "settings.json", v) }, false)
}

func loadSettings(load func(any) error, allowMissing bool) (Settings, error) {
	var value struct {
		Schema       string `json:"schema"`
		StartAtLogin *bool  `json:"startAtLogin"`
		AutoUpdate   *bool  `json:"autoUpdate"`
	}
	err := load(&value)
	if allowMissing && errors.Is(err, os.ErrNotExist) {
		return Settings{Schema: Schema, StartAtLogin: true, AutoUpdate: true}, nil
	}
	if err != nil {
		return Settings{}, errors.Join(errors.New("invalid app settings"), err)
	}
	if value.Schema != Schema || value.StartAtLogin == nil || value.AutoUpdate == nil {
		return Settings{}, errors.New("invalid app settings")
	}
	return Settings{Schema: value.Schema, StartAtLogin: *value.StartAtLogin, AutoUpdate: *value.AutoUpdate}, nil
}
func Save(base string, value Settings) error {
	if value.Schema != Schema {
		return errors.New("invalid app settings")
	}
	preferences, err := LockPreferences(base)
	if err != nil {
		return err
	}
	defer preferences.Close()
	return preferences.Save(value)
}

func saveSettings(base string, value Settings) error {
	if value.Schema != Schema {
		return errors.New("invalid app settings")
	}
	gate.Lock()
	defer gate.Unlock()
	return write(base, "settings.json", value)
}
func LoadResume(base string) (Resume, error) {
	return loadResume(func(v any) error { return read(base, "resume.json", v) }, true)
}

func loadResume(load func(any) error, allowMissing bool) (Resume, error) {
	var wire struct {
		Alias       *string         `json:"alias"`
		Fingerprint *string         `json:"fingerprint"`
		ManualStop  json.RawMessage `json:"manualStop"`
	}
	err := load(&wire)
	if allowMissing && errors.Is(err, os.ErrNotExist) {
		return Resume{}, nil
	}
	if err != nil {
		if errors.Is(err, errInvalidDocument) {
			return Resume{}, errors.Join(ErrInvalidResume, err)
		}
		return Resume{}, err
	}
	if wire.Alias == nil || wire.Fingerprint == nil {
		return Resume{}, fmt.Errorf("%w: missing peer fields", ErrInvalidResume)
	}
	v := Resume{Alias: *wire.Alias, Fingerprint: *wire.Fingerprint}
	if len(wire.ManualStop) > 0 {
		var stop *bool
		if err := json.Unmarshal(wire.ManualStop, &stop); err != nil || stop == nil {
			return Resume{}, fmt.Errorf("%w: invalid manual stop", ErrInvalidResume)
		}
		v.ManualStop = *stop
	}
	if v.ManualStop && (v.Alias != "" || v.Fingerprint != "") {
		return Resume{}, fmt.Errorf("%w: manual stop contains a resume peer", ErrInvalidResume)
	}
	if v.Alias == "" && v.Fingerprint == "" {
		return v, nil
	}
	if len(v.Alias) > 256 || v.Alias == "" || len(v.Fingerprint) != 64 {
		return Resume{}, fmt.Errorf("%w: invalid resume peer", ErrInvalidResume)
	}
	if _, err = hex.DecodeString(v.Fingerprint); err != nil {
		return Resume{}, errors.Join(ErrInvalidResume, err)
	}
	return v, nil
}
func SaveResume(base string, v Resume) error {
	if v.ManualStop && (v.Alias != "" || v.Fingerprint != "") {
		return errors.New("manual stop contains a resume peer")
	}
	if v.Alias == "" && v.Fingerprint != "" {
		return errors.New("invalid resume peer")
	}
	if v.Alias != "" {
		if len(v.Alias) > 256 || len(v.Fingerprint) != 64 {
			return errors.New("invalid resume peer")
		}
		if _, err := hex.DecodeString(v.Fingerprint); err != nil {
			return err
		}
	}
	gate.Lock()
	defer gate.Unlock()
	return write(base, "resume.json", v)
}
func read(base, name string, value any) error {
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return err
	}
	path := filepath.Join(paths.Root, "launch")
	if _, err = os.Lstat(path); err != nil {
		return err
	}
	if _, err = peercredentials.ValidateSecurityDirectory(path); err != nil {
		return err
	}
	// The scheduled recovery CLI runs from this folder. A read must not
	// request DELETE/WRITE_DAC or create state through PrepareLaunch.
	dir, err := securefs.OpenReadDirectory(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	file, err := dir.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = peercredentials.ValidateSecurityFileHandle(file); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return err
	}
	if len(data) > 4096 {
		return fmt.Errorf("%w: settings too large", errInvalidDocument)
	}
	return decodeSettings(data, value)
}

func decodeSettings(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return errors.Join(errInvalidDocument, err)
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("%w: extra settings data", errInvalidDocument)
	}
	return nil
}
func write(base, name string, value any) error {
	return writeMode(base, name, value, true)
}

func writeMode(base, name string, value any, replace bool) error {
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, temp, err := dir.CreateTemporary(".settings-", 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(temp)
	if err = peercredentials.ProtectSecurityFileHandle(file); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if replace {
		return dir.Replace(temp, name)
	}
	return dir.PromoteNoReplace(temp, name)
}
