package productlog

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestCleanupReceiptIsReadOnlyAndExported(t *testing.T) {
	base := t.TempDir()
	if receipt, err := CleanupReceipt(base); err != nil || receipt != nil {
		t.Fatal(receipt, err)
	}
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) != 0 {
		t.Fatal("read-only query created files", err)
	}
	want := CleanupSummary{Time: time.Now().UTC().Format(time.RFC3339Nano), Code: "failed", Removed: 2}
	if err := SaveCleanup(base, want); err != nil {
		t.Fatal(err)
	}
	got, err := CleanupReceipt(base)
	if err != nil || got == nil || *got != want {
		t.Fatal(got, err)
	}
	path, err := Export(base)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cleanup *CleanupSummary `json:"cleanup"`
	}
	if err = json.Unmarshal(data, &doc); err != nil || doc.Cleanup == nil || *doc.Cleanup != want {
		t.Fatal("cleanup receipt lost", err)
	}
	if err := SaveCleanup(base, CleanupSummary{Time: want.Time, Code: "private-path", Removed: 2}); err == nil {
		t.Fatal("unbounded code accepted")
	}
}
