//go:build windows

package launcherui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestClipboardFilePackerChild(t *testing.T) {
	switch os.Getenv("KRASIS_TEST_PACK_CHILD") {
	case "pack":
		os.Exit(RunClipboardFilePacker(os.Stdin, os.Stdout))
	case "blocked":
		time.Sleep(time.Hour)
		os.Exit(1)
	}
}

func TestClipboardPackChildRoundtripAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("copy"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, blocked := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		mode := "pack"
		if blocked {
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), 150*time.Millisecond)
			mode = "blocked"
		}
		self, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(ctx, self, "-test.run=^TestClipboardFilePackerChild$")
		command.Env = append(os.Environ(), "KRASIS_TEST_PACK_CHILD="+mode)
		started := time.Now()
		data, err := runClipboardFilePacker(ctx, command, []string{path})
		if blocked {
			if err == nil || len(data) != 0 || time.Since(started) > 2*time.Second {
				t.Fatal("blocked helper escaped cancellation", err)
			}
		} else if err != nil || len(data) == 0 {
			t.Fatal("pack failed", err)
		}
		cancel()
	}
}
