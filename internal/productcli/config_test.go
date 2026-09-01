package productcli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveControlDirectoryPrecedence(t *testing.T) {
	localAppData := t.TempDir()
	root := filepath.Join(localAppData, "Krasis")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	configured := filepath.Join(localAppData, "configured-control")
	data := `{"schemaVersion":"krasis.cli-config/v1","controlDirectory":"` + filepath.ToSlash(configured) + `"}`
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveControlDirectory(localAppData, "")
	if err != nil || resolved != filepath.Clean(configured) {
		t.Fatalf("ResolveControlDirectory() = %q, %v", resolved, err)
	}
	override := filepath.Join(localAppData, "override-control")
	resolved, err = ResolveControlDirectory(localAppData, override)
	if err != nil || resolved != filepath.Clean(override) {
		t.Fatalf("override = %q, %v", resolved, err)
	}
}

func TestResolveControlDirectoryRejectsUnknownAndTrailingConfig(t *testing.T) {
	for _, payload := range []string{
		`{"schemaVersion":"krasis.cli-config/v1","unknown":true}`,
		`{"schemaVersion":"krasis.cli-config/v1"} {}`,
	} {
		localAppData := t.TempDir()
		root := filepath.Join(localAppData, "Krasis")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveControlDirectory(localAppData, ""); err == nil {
			t.Fatal("invalid config was accepted")
		}
	}
}
