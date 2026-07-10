package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrOutputExists = errors.New("evidence output already exists")

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

	temporary, err := os.CreateTemp(directory, ".krasis-probe-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary evidence file: %w", err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := false
	defer func() {
		if !keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

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
	if err := promoteNoReplace(temporaryPath, absolute); err != nil {
		if errors.Is(err, ErrOutputExists) {
			return err
		}
		return fmt.Errorf("promote evidence file: %w", err)
	}
	keepTemporary = true
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
	if _, err := os.Lstat(absolute); err == nil {
		return "", "", fmt.Errorf("%w: %s", ErrOutputExists, absolute)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("inspect output path: %w", err)
	}
	return absolute, directory, nil
}
