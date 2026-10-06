package appsettings

import (
	"os"
	"strings"
	"testing"
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
