package productcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/0disoft/krasis/internal/productidentity"
)

const (
	ConfigSchemaVersion = "krasis.cli-config/v1"
	configFilename      = "config.json"
	maxConfigBytes      = 64 << 10
)

type Config struct {
	SchemaVersion    string `json:"schemaVersion"`
	ControlDirectory string `json:"controlDirectory,omitempty"`
}

func ResolveControlDirectory(localAppData, override string) (string, error) {
	paths, err := productidentity.ResolveWindowsPaths(localAppData)
	if err != nil {
		return "", err
	}
	if override != "" {
		return validateControlDirectory(override)
	}
	config, err := loadConfig(filepath.Join(paths.Root, configFilename))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err == nil && config.ControlDirectory != "" {
		return validateControlDirectory(config.ControlDirectory)
	}
	return paths.Control, nil
}

func loadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return Config{}, err
	}
	if len(data) > maxConfigBytes {
		return Config{}, errors.New("CLI config exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, errors.New("CLI config is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("CLI config contains trailing JSON")
	}
	if config.SchemaVersion != ConfigSchemaVersion {
		return Config{}, errors.New("CLI config schema is unsupported")
	}
	if config.ControlDirectory != "" {
		if _, err := validateControlDirectory(config.ControlDirectory); err != nil {
			return Config{}, err
		}
	}
	return config, nil
}

func validateControlDirectory(value string) (string, error) {
	if strings.TrimSpace(value) != value || value == "" || !filepath.IsAbs(value) {
		return "", errors.New("control directory must be an absolute path without surrounding whitespace")
	}
	return filepath.Clean(value), nil
}
