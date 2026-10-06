package productcli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestRelaunchDoesNotNeedAgentOrAllowPathArguments(t *testing.T) {
	base, bundle := t.TempDir(), t.TempDir()
	self := filepath.Join(bundle, "krasis.exe")
	if err := os.WriteFile(self, []byte("MZ-test"), 0600); err != nil {
		t.Fatal(err)
	}
	count := 0
	deps := dependencies{localAppData: base, executable: filepath.Join(bundle, "krasis-cli.exe"),
		openClient: func(string) (apiClient, error) { t.Fatal("opened agent"); return nil, localapi.ErrUnavailable },
		launch: func(path string) error {
			if path != self {
				t.Fatal("wrong path")
			}
			count++
			return nil
		},
	}
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"relaunch", "--json"}, &out, &errOut, deps); code != ExitOK || !strings.Contains(out.String(), "launch_requested") || count != 1 {
		t.Fatalf("code=%d out=%s", code, out.String())
	}
	for _, args := range [][]string{{"relaunch", "other.exe"}, {"relaunch", "--control-dir", base}} {
		if code := run(context.Background(), args, &out, &errOut, deps); code != ExitUsage || count != 1 {
			t.Fatalf("unexpected launch: %v", args)
		}
	}
	if code := run(context.Background(), []string{"relaunch", "--startup", "--json"}, &out, &errOut, deps); code != ExitOK || count != 2 {
		t.Fatalf("startup launch: %d", code)
	}
	if _, err := parseGlobalOptions([]string{"status", "--startup"}); err != nil {
		t.Fatal(err)
	}
	if err := validateCommandArgs([]string{"status", "--startup"}); err == nil {
		t.Fatal("startup flag accepted for status")
	}
}
