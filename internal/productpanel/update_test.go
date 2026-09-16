package productpanel

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/updatebundle"
)

func TestUpdateResumeRequiresConfirmedRuntimeAndLocalState(t *testing.T) {
	for _, local := range []string{"LOCAL", "CONNECTED", "CONNECTING", "FAULTED"} {
		for _, update := range []string{"DELIVERED", "TRANSFER_UNKNOWN", "RESTART_UNCONFIRMED", "PEER_APPLY_FAILED", "UPDATED"} {
			want := local == "LOCAL" && update == "UPDATED"
			if CanResumeAfterUpdate(local, peerupdate.Status{State: update}) != want {
				t.Fatalf("resume for local=%s update=%s", local, update)
			}
		}
	}
}

func TestUpdateDeliveryIsNotReportedAsInstalled(t *testing.T) {
	if UpdateNotice("DELIVERED") == "" || UpdateNotice("TRANSFER_UNKNOWN") == "" || UpdateNotice("PEER_NOT_ALLOWED") == "" {
		t.Fatal("missing update outcome")
	}
	if UpdateNotice("private path") != "" {
		t.Fatal("untrusted state exposed")
	}
}

// Opt-in packaging check: inspect real PE/build metadata, never execute updates.
func TestPackagedUpdateBundle(t *testing.T) {
	directory := os.Getenv("KRASIS_UPDATE_SMOKE")
	if directory == "" {
		t.Skip("runs only from the configured package builder")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	archive, err := root.Open("krasis-windows-x64-smoke.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	info, err := archive.Stat()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := updatebundle.Read(archive, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Metadata().Generation != localapi.AgentGeneration || bundle.Metadata().Revision != os.Getenv("KRASIS_UPDATE_REVISION") {
		t.Fatal("packaged identity mismatch")
	}
	data, err := updatebundle.Pack(root)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := updatebundle.Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	target, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if err := copy.Stage(target, strings.Repeat("a", 32)); err != nil {
		t.Fatal(err)
	}
}
