//go:build windows

package launcherui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClipboardCleanupQuarantinesBoundedBatchBeforeDeletion(t *testing.T) {
	base := t.TempDir()
	for i := 0; i < 10; i++ {
		stage := filepath.Join(base, fmt.Sprintf("copy-%02d", i))
		if err := os.Mkdir(stage, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stage, "note.txt"), []byte("paste"), 0600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-48 * time.Hour)
		if err := os.Chtimes(stage, old, old); err != nil {
			t.Fatal(err)
		}
	}
	active := filepath.Join(base, "copy-00")
	trash := quarantineClipboardCandidates(base, []string{active}, clipboardCleanupCandidates(base))
	if len(trash) == 0 || len(trash) > maxClipboardCleanupBatch {
		t.Fatal("unbounded or empty cleanup", len(trash))
	}
	for _, path := range trash {
		if _, err := os.Stat(filepath.Join(path, "note.txt")); err != nil {
			t.Fatal("recursive deletion occurred while quarantining", err)
		}
	}
	if _, err := os.Stat(filepath.Join(active, "note.txt")); err != nil {
		t.Fatal("active paste removed", err)
	}
	// A process restart can discover quarantined folders without an in-memory list.
	queued := clipboardCleanupCandidates(base)
	if len(queued) == 0 {
		t.Fatal("pending deletion not discoverable")
	}
	removeClipboardTrash(trash)
	for _, path := range trash {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("quarantine not removed", err)
		}
	}
}
