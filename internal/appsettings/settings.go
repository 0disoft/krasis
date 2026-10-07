// Package appsettings owns per-user startup and release-update preferences.
package appsettings

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

const Schema = "krasis.app-settings/v1"

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

var gate sync.Mutex

func Load(base string) (Settings, error) {
	var value struct {
		Schema       string `json:"schema"`
		StartAtLogin *bool  `json:"startAtLogin"`
		AutoUpdate   *bool  `json:"autoUpdate"`
	}
	err := read(base, "settings.json", &value)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{Schema: Schema, StartAtLogin: true, AutoUpdate: true}, nil
	}
	if err != nil || value.Schema != Schema || value.StartAtLogin == nil || value.AutoUpdate == nil {
		return Settings{}, errors.New("invalid app settings")
	}
	return Settings{Schema: value.Schema, StartAtLogin: *value.StartAtLogin, AutoUpdate: *value.AutoUpdate}, nil
}
func Save(base string, value Settings) error {
	if value.Schema != Schema {
		return errors.New("invalid app settings")
	}
	gate.Lock()
	defer gate.Unlock()
	return write(base, "settings.json", value)
}
func LoadResume(base string) (Resume, error) {
	var v Resume
	err := read(base, "resume.json", &v)
	if errors.Is(err, os.ErrNotExist) {
		return v, nil
	}
	if err != nil {
		return Resume{}, err
	}
	if v.ManualStop && (v.Alias != "" || v.Fingerprint != "") {
		return Resume{}, errors.New("manual stop contains a resume peer")
	}
	if v.Alias == "" && v.Fingerprint == "" {
		return v, nil
	}
	if len(v.Alias) > 256 || v.Alias == "" || len(v.Fingerprint) != 64 {
		return Resume{}, errors.New("invalid resume peer")
	}
	if _, err = hex.DecodeString(v.Fingerprint); err != nil {
		return Resume{}, err
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
		return errors.New("settings too large")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(value); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("extra settings data")
	}
	return nil
}
func write(base, name string, value any) error {
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
	return dir.Replace(temp, name)
}
