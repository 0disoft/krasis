package appsettings

import (
	"errors"
	"os"
	"testing"
)

func TestStartupInitializesDefaultsOnlyWhenSettingsAreAbsent(t *testing.T) {
	base := t.TempDir()
	if _, err := Load(base); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(base); len(entries) != 0 {
		t.Fatal("read-only load persisted defaults")
	}
	got, err := Initialize(base)
	if err != nil || !got.StartAtLogin || !got.AutoUpdate {
		t.Fatal(got, err)
	}
	disabled := Settings{Schema: Schema}
	if err := Save(base, disabled); err != nil {
		t.Fatal(err)
	}
	if got, err = Initialize(base); err != nil || got != disabled {
		t.Fatal("startup reset disabled preference", got, err)
	}
	if err := writeMode(base, "settings.json", Settings{Schema: Schema, StartAtLogin: true, AutoUpdate: true}, false); !errors.Is(err, os.ErrExist) {
		t.Fatal("initialization could overwrite concurrent settings", err)
	}
	if got, err = Load(base); err != nil || got != disabled {
		t.Fatal("no-replace changed settings", got, err)
	}
}

func TestInitializationNeverRepairsMalformedExistingSettings(t *testing.T) {
	for _, raw := range []string{"{}", "null", "{", `{"schema":"krasis.app-settings/v1","startAtLogin":false}`} {
		base := t.TempDir()
		writeRawSettings(t, base, "settings.json", raw)
		if _, err := Initialize(base); err == nil {
			t.Fatal("malformed settings replaced", raw)
		}
		if _, err := Load(base); err == nil {
			t.Fatal("damaged policy now allowed", raw)
		}
	}
}
