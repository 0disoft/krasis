package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0disoft/krasis/internal/securefs"
)

var (
	ErrOutputExists     = errors.New("evidence output already exists")
	ErrUnsafeOutputPath = errors.New("evidence output path is unsafe")
)

func ValidateJSONOutputPath(path string) error {
	_, _, err := resolveJSONOutputPath(path)
	return err
}

func WriteJSONExclusive(path string, value any) error {
	absolute, directory, err := resolveJSONOutputPath(path)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode evidence JSON: %w", err)
	}
	data = append(data, '\n')

	secureDirectory, err := securefs.OpenDirectory(directory)
	if err != nil {
		return fmt.Errorf("open secure evidence directory: %w", err)
	}
	defer secureDirectory.Close()
	temporary, temporaryName, err := secureDirectory.CreateTemporary(".krasis-probe-", 0o600)
	if err != nil {
		return fmt.Errorf("create temporary evidence file: %w", err)
	}
	defer secureDirectory.Remove(temporaryName)

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("restrict temporary evidence file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary evidence file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("flush temporary evidence file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary evidence file: %w", err)
	}
	if err := validateOutputDirectory(directory); err != nil {
		return err
	}
	if err := secureDirectory.PromoteNoReplace(temporaryName, filepath.Base(absolute)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: output already exists", ErrOutputExists)
		}
		return fmt.Errorf("promote evidence file: %w", err)
	}
	return nil
}

func resolveJSONOutputPath(path string) (string, string, error) {
	if strings.TrimSpace(path) == "" {
		return "", "", fmt.Errorf("output path is required")
	}
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return "", "", fmt.Errorf("output path must use the .json extension")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("resolve output path: %w", err)
	}
	directory := filepath.Dir(absolute)
	info, err := os.Stat(directory)
	if err != nil {
		return "", "", fmt.Errorf("inspect output directory: %w", err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("output parent is not a directory")
	}
	if err := validateOutputDirectory(directory); err != nil {
		return "", "", err
	}
	if _, err := os.Lstat(absolute); err == nil {
		return "", "", fmt.Errorf("%w: %s", ErrOutputExists, absolute)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect output path: %w", err)
	}
	return absolute, directory, nil
}
