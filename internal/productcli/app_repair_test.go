package productcli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/launchrecord"
)

func TestAppRepairRejectsArgumentsAndInvalidBundleWithoutAgent(t *testing.T) {
	base, bundle := t.TempDir(), t.TempDir()
	deps := dependencies{localAppData: base, executable: filepath.Join(bundle, "krasis-cli.exe"),
		openClient: func(string) (apiClient, error) { t.Fatal("repair contacted agent"); return nil, nil },
		launch:     func(string) error { t.Fatal("repair launched UI"); return nil },
	}
	for _, args := range [][]string{{"app", "repair", "other.exe"}, {"app", "repair", "B"}, {"app", "repair", "--control-dir", base}} {
		var out, errs bytes.Buffer
		if code := run(context.Background(), args, &out, &errs, deps); code != ExitUsage {
			t.Fatalf("accepted %v: %d", args, code)
		}
	}
	for _, want := range []string{"bundle_missing", "bundle_invalid"} {
		var out, errs bytes.Buffer
		if code := run(context.Background(), []string{"app", "repair", "--json"}, &out, &errs, deps); code != ExitUnsafeState || !strings.Contains(out.String(), want) || strings.Contains(out.String(), bundle) {
			t.Fatalf("code=%d output=%s", code, out.String())
		}
		if _, err := launchrecord.Load(base); err != launchrecord.ErrMissing {
			t.Fatal("invalid bundle published recovery", err)
		}
		if err := os.WriteFile(filepath.Join(bundle, "BUILD.json"), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAppRepairRegistersOnlyMatchingVerifiedBuild(t *testing.T) {
	base, directory := t.TempDir(), t.TempDir()
	cli := filepath.Join(directory, "krasis-cli.exe")
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("MZ-test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	version := versionResult{CLIGeneration: 135, CLIBuild: strings.Repeat("a", 40)}
	bundle := updateVerifyResult{Valid: true, Generation: 135, Revision: version.CLIBuild}
	for _, invalid := range []updateVerifyResult{
		{Valid: false, Generation: 135, Revision: version.CLIBuild},
		{Valid: true, Generation: 134, Revision: version.CLIBuild},
		{Valid: true, Generation: 135, Revision: strings.Repeat("b", 40)},
		{Valid: true, Generation: 135, Revision: "unknown"},
	} {
		if result, code := registerAppRecovery(base, cli, invalid, version); code != ExitUnsafeState || result.Code != "bundle_version_mismatch" {
			t.Fatal("mismatched build accepted", result)
		}
	}
	if result, code := registerAppRecovery(base, cli, bundle, version); code != ExitOK || result.Code != "recovery_ready" {
		t.Fatal(result)
	}
	record, err := launchrecord.Inspect(base)
	if err != nil || record.Generation != 135 || record.Revision != version.CLIBuild || record.Executable != filepath.Join(directory, "krasis.exe") {
		t.Fatal("incorrect recovery target", err)
	}
	if err := os.Remove(cli); err != nil {
		t.Fatal(err)
	}
	if result, code := registerAppRecovery(base, cli, bundle, version); code != ExitUnsafeState || result.Stage != "cli" {
		t.Fatal("registration failure unreported", result)
	}
}
