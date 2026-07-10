package evidence

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteJSONExclusiveWritesFinalNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe.json")

	if err := WriteJSONExclusive(path, map[string]string{"schemaVersion": "test/v1"}); err != nil {
		t.Fatalf("WriteJSONExclusive returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("evidence does not end with newline: %q", string(data))
	}
}

func TestWriteJSONExclusiveDoesNotReplaceExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe.json")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("seed output: %v", err)
	}

	err := WriteJSONExclusive(path, map[string]string{"value": "replacement"})
	if !errors.Is(err, ErrOutputExists) {
		t.Fatalf("error = %v, want ErrOutputExists", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read existing output: %v", readErr)
	}
	if string(data) != "original" {
		t.Fatalf("existing output changed: %q", string(data))
	}
}

func TestWriteJSONExclusiveRequiresJSONExtension(t *testing.T) {
	err := WriteJSONExclusive(filepath.Join(t.TempDir(), "probe.txt"), map[string]string{})
	if err == nil {
		t.Fatal("WriteJSONExclusive accepted a non-JSON extension")
	}
}

func TestValidateJSONOutputPathRejectsMissingParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "probe.json")
	if err := ValidateJSONOutputPath(path); err == nil {
		t.Fatal("ValidateJSONOutputPath accepted a missing parent directory")
	}
}
