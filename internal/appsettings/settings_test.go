package appsettings

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
)

func TestDefaultsAndDisabledSurviveReload(t *testing.T) {
	base := t.TempDir()
	v, err := Load(base)
	if err != nil || !v.StartAtLogin || !v.AutoUpdate {
		t.Fatalf("%+v %v", v, err)
	}
	if entries, err := os.ReadDir(base); err != nil || len(entries) != 0 {
		t.Fatal("reading defaults created state", err)
	}
	v.StartAtLogin = false
	v.AutoUpdate = false
	if err = Save(base, v); err != nil {
		t.Fatal(err)
	}
	got, err := Load(base)
	if err != nil || got.StartAtLogin || got.AutoUpdate {
		t.Fatalf("%+v %v", got, err)
	}
	peer := Resume{Alias: "B", Fingerprint: strings.Repeat("a", 64)}
	if err = SaveResume(base, peer); err != nil {
		t.Fatal(err)
	}
	gotPeer, err := LoadResume(base)
	if err != nil || gotPeer != peer {
		t.Fatalf("%+v %v", gotPeer, err)
	}
	if err = SaveResume(base, Resume{}); err != nil {
		t.Fatal(err)
	}
	gotPeer, err = LoadResume(base)
	if err != nil || gotPeer.Alias != "" {
		t.Fatal("disconnect not retained")
	}
}

func TestPersistentManualStopCannotCarryAResumePeer(t *testing.T) {
	base := t.TempDir()
	stop := Resume{ManualStop: true}
	if err := SaveResume(base, stop); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadResume(base); err != nil || got != stop {
		t.Fatal(got, err)
	}
	if err := SaveResume(base, Resume{ManualStop: true, Alias: "B", Fingerprint: strings.Repeat("a", 64)}); err == nil {
		t.Fatal("stop and reconnect intent accepted together")
	}
}

func TestIncompleteResumeIsNotANewInstallationOrAnAllowedResume(t *testing.T) {
	for _, raw := range []string{"{}", "null", "{", `{"manualStop":true}`, `{"alias":""}`, `{"alias":null,"fingerprint":""}`, `{"alias":"","fingerprint":null}`, `{"alias":"","fingerprint":"","manualStop":null}`, `{"alias":"","fingerprint":"","manualStop":"false"}`} {
		t.Run(raw, func(t *testing.T) {
			base := t.TempDir()
			writeRawSettings(t, base, "resume.json", raw)
			if _, err := LoadResume(base); !errors.Is(err, ErrInvalidResume) {
				t.Fatal("incomplete saved resume accepted")
			}
		})
	}
	for _, raw := range []string{`{"alias":"","fingerprint":""}`, `{"alias":"B","fingerprint":"` + strings.Repeat("a", 64) + `"}`} {
		base := t.TempDir()
		writeRawSettings(t, base, "resume.json", raw)
		if _, err := LoadResume(base); err != nil {
			t.Fatal("legacy complete resume rejected", err)
		}
	}
	if value, err := LoadResume(t.TempDir()); err != nil || value != (Resume{}) {
		t.Fatal("absent initial file rejected", value, err)
	}
}

func TestIncompleteSettingsNeverEnableStartupOrUpdates(t *testing.T) {
	for _, raw := range []string{"", "{}", "null", `{"schema":"krasis.app-settings/v1"}`, `{"schema":"krasis.app-settings/v1","startAtLogin":false}`, `{"schema":"krasis.app-settings/v1","autoUpdate":false}`, `{"schema":"krasis.app-settings/v1","startAtLogin":null,"autoUpdate":false}`, `{"schema":"krasis.app-settings/v1","startAtLogin":false,"autoUpdate":null}`} {
		t.Run(raw, func(t *testing.T) {
			base := t.TempDir()
			writeRawSettings(t, base, "settings.json", raw)
			got, err := Load(base)
			if err == nil || got.StartAtLogin || got.AutoUpdate {
				t.Fatalf("incomplete settings became defaults: %+v %v", got, err)
			}
		})
	}
}

func TestIncompleteRecoverySettingsNeverResumeRecovery(t *testing.T) {
	for _, raw := range []string{"", "{}", "null", `{"paused":null}`} {
		t.Run(raw, func(t *testing.T) {
			base := t.TempDir()
			writeRawSettings(t, base, "recovery.json", raw)
			if _, err := RecoveryPaused(base); err == nil {
				t.Fatal("incomplete recovery settings accepted")
			}
		})
	}
	base := t.TempDir()
	for _, paused := range []bool{true, false} {
		if err := PauseRecovery(base, paused); err != nil {
			t.Fatal(err)
		}
		if got, err := RecoveryPaused(base); err != nil || got != paused {
			t.Fatal(got, err)
		}
	}
}

func writeRawSettings(t *testing.T, base, name, raw string) {
	t.Helper()
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	f, err := dir.CreateExclusive(name, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(raw); err != nil {
		t.Fatal(err)
	}
}
