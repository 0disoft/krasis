//go:build windows && desktop

package workspacehost

import (
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
)

func TestDiagnosticFilesSeparatesLocalAndReceivedWithoutExport(t *testing.T) {
	base := t.TempDir()
	local, remote, err := diagnosticFiles(base)
	if err != nil || local || remote {
		t.Fatalf("empty: %v %v %v", local, remote, err)
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	for _, name := range []string{productlog.ExportName, "support-peer-invalid.json"} {
		f, err := dir.CreateExclusive(name, 0600)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	local, remote, err = diagnosticFiles(base)
	if err != nil || !local || remote {
		t.Fatalf("local only: %v %v %v", local, remote, err)
	}
	f, err := dir.CreateExclusive("support-peer-"+strings.Repeat("a", 64)+".json", 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	local, remote, err = diagnosticFiles(base)
	if err != nil || !local || !remote {
		t.Fatalf("both: %v %v %v", local, remote, err)
	}
}
