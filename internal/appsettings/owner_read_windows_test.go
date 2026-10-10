//go:build windows

package appsettings

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestOwnerStartupRequiresExistingPolicyAndReadsManualStop(t *testing.T) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	owner := user.User.Sid.String()
	base := t.TempDir()
	if _, _, err := ReadOwnerStartup(base, owner); err == nil {
		t.Fatal("missing boot policy got defaults")
	}
	settings := Settings{Schema: Schema, StartAtLogin: true, AutoUpdate: true}
	if err := Save(base, settings); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadOwnerStartup(base, owner); err == nil {
		t.Fatal("missing resume accepted")
	}
	resume := Resume{Alias: "B", Fingerprint: strings.Repeat("a", 64)}
	if err := SaveResume(base, resume); err != nil {
		t.Fatal(err)
	}
	s, r, err := ReadOwnerStartup(base, owner)
	if err != nil || s != settings || r != resume {
		t.Fatal("policy snapshot changed", err)
	}
	if err := SaveResume(base, Resume{ManualStop: true}); err != nil {
		t.Fatal(err)
	}
	_, r, err = ReadOwnerStartup(base, owner)
	if err != nil || !r.ManualStop || r.Alias != "" {
		t.Fatal("manual stop lost", err)
	}
}
