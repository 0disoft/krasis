//go:build windows

package productlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productidentity"
)

func TestExportReadsLogsWhileWriterOwnsDirectory(t *testing.T) {
	base := t.TempDir()
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	l := newLogger(dir, closer{}, "agent")
	defer l.Close()
	if err := l.write(record{Time: time.Now().UTC().Format(time.RFC3339Nano), Code: "agent_ready"}); err != nil {
		t.Fatal(err)
	}
	path, err := Export(base)
	if err != nil {
		t.Fatal(err)
	}
	// Close the writer's directory handle before ordinary path-based inspection.
	l.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Logs []exportLog }
	if json.Unmarshal(data, &doc) != nil || len(doc.Logs) != 4 || len(doc.Logs[1].Events) != 1 || doc.Logs[1].Incomplete {
		t.Fatal("active log was not exported")
	}
}

func TestExportOfflineReplacesSnapshotWithoutReadingOtherFiles(t *testing.T) {
	base := t.TempDir()
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	f, err := dir.CreateExclusive("unrelated.txt", 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("private-marker")
	f.Close()
	dir.Close()
	for i := 0; i < 2; i++ {
		path, err := Export(base)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Schema string
			Logs   []exportLog
		}
		if json.Unmarshal(data, &doc) != nil || doc.Schema != "krasis.support/v1" || len(doc.Logs) != 4 {
			t.Fatal("invalid export")
		}
		for _, log := range doc.Logs {
			if !log.Incomplete || len(log.Events) != 0 {
				t.Fatal("offline logs not reported")
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join(base, "Krasis", "diagnostics"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("unexpected retention or temp leak: %v %v", entries, err)
	}
}
