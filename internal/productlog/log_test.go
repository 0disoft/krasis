package productlog

import (
	"encoding/json"
	"github.com/0disoft/krasis/internal/securefs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type closer struct{}

func (closer) Close() error { return nil }

func TestPersistencePrivacyAndConcurrentClose(t *testing.T) {
	path := t.TempDir()
	dir, err := securefs.OpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	l := newLogger(dir, closer{}, "agent")
	l.Event("process_started")
	l.Event("secret-key-contents")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); l.Event("CONNECTED"); l.Close() }()
	}
	wg.Wait()
	data, err := os.ReadFile(filepath.Join(path, "agent.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "process_started") || strings.Contains(string(data), "secret") {
		t.Fatalf("unexpected log: %s", data)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var r record
		if err := json.Unmarshal([]byte(line), &r); err != nil || r.Time == "" {
			t.Fatal("invalid record")
		}
	}
}

func TestRotationAndWriteFailure(t *testing.T) {
	path := t.TempDir()
	dir, err := securefs.OpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	l := &Logger{dir: dir, role: "agent"}
	for i := 0; i < 3; i++ {
		f, err := dir.CreateExclusive("agent.jsonl", 0600)
		if os.IsExist(err) {
			f, err = dir.OpenReadWrite("agent.jsonl")
		}
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Truncate(maxBytes); err != nil {
			t.Fatal(err)
		}
		f.Close()
		if err = l.write(record{Code: "process_started"}); err != nil {
			t.Fatal(err)
		}
	}
	// Release the security directory handle before opening it through os.ReadDir.
	dir.Close()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("retention exceeded: %v", entries)
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > maxBytes {
			t.Fatal("size exceeded")
		}
	}
	dir.Close()
	if l.write(record{Code: "process_started"}) == nil {
		t.Fatal("write failure hidden")
	}
}
