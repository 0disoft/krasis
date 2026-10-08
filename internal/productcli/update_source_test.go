package productcli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/updatebundle"
)

func recordedUpdateCLI(t *testing.T) (string, string, string) {
	t.Helper()
	base, bundle := t.TempDir(), t.TempDir()
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte("fixture "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	revision := strings.Repeat("a", 40)
	data, _ := json.Marshal(updatebundle.Metadata{Generation: 245, Revision: revision})
	if err := os.WriteFile(filepath.Join(bundle, "BUILD.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := launchrecord.Register(base, filepath.Join(bundle, "krasis.exe"), 245, revision); err != nil {
		t.Fatal(err)
	}
	dir, err := launchrecord.Directory(base)
	if err != nil {
		t.Fatal(err)
	}
	return base, bundle, filepath.Join(dir, "krasis-cli.exe")
}

func TestFixedCLISelectsRecordedBundleAndVerifyStaysOffline(t *testing.T) {
	base, bundle, fixed := recordedUpdateCLI(t)
	client := updateFixture(strings.Repeat("b", 64), 242)
	deps := testDependencies(client)
	deps.localAppData, deps.executable = base, fixed
	var out, errs bytes.Buffer
	if code := run(context.Background(), []string{"update", "send", "B", "--json"}, &out, &errs, deps); code != ExitOK || errs.Len() != 0 {
		t.Fatal(code, out.String(), errs.String())
	}
	request := client.requests[len(client.requests)-1]
	if request.Action != "SEND" || request.Directory != bundle {
		t.Fatal("fixed CLI did not send from the recorded bundle", request)
	}
	out.Reset()
	deps.openClient = func(string) (apiClient, error) { t.Fatal("offline verification contacted agent"); return nil, nil }
	// Fixture is intentionally incomplete: finding the recorded source must
	// still invoke full bundle validation rather than advertise success.
	if code := run(context.Background(), []string{"update", "verify", "--json"}, &out, &errs, deps); code != ExitUnsafeState || !strings.Contains(out.String(), `"code":"bundle_invalid"`) {
		t.Fatal(code, out.String())
	}
}

func TestFixedCLIRejectsChangedRegistrationOrBytesBeforeSending(t *testing.T) {
	for _, changed := range []string{"ui", "source cli", "fixed cli", "manifest", "missing record"} {
		t.Run(changed, func(t *testing.T) {
			base, bundle, fixed := recordedUpdateCLI(t)
			path := filepath.Join(bundle, "krasis.exe")
			switch changed {
			case "source cli":
				path = filepath.Join(bundle, "krasis-cli.exe")
			case "fixed cli":
				path = fixed
			case "manifest":
				path = filepath.Join(bundle, "BUILD.json")
			case "missing record":
				path = filepath.Join(filepath.Dir(fixed), "current.json")
			}
			if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			if changed == "missing record" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			client := updateFixture(strings.Repeat("b", 64), 242)
			deps := testDependencies(client)
			deps.localAppData, deps.executable = base, fixed
			var out, errs bytes.Buffer
			if code := run(context.Background(), []string{"update", "send", "B", "--json"}, &out, &errs, deps); code != ExitUnsafeState || !strings.Contains(out.String(), `"code":"bundle_source_invalid"`) || len(client.requests) != 0 {
				t.Fatal("changed source sent an update", code, out.String(), client.requests)
			}
		})
	}
}

func TestPortableCLIDoesNotFollowAnotherRecordedImage(t *testing.T) {
	base, _, _ := recordedUpdateCLI(t)
	portable := filepath.Join(t.TempDir(), "krasis-cli.exe")
	if got, err := updateBundleExecutable(base, portable); err != nil || got != portable {
		t.Fatal("portable source was replaced", got, err)
	}
}
