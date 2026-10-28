//go:build windows

package peerupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

func currentUIStage(t *testing.T, root, id string, generation int, revision string) string {
	t.Helper()
	path := filepath.Join(root, id)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(updatebundle.Metadata{Generation: generation, Revision: revision})
	if err := os.WriteFile(filepath.Join(path, "BUILD.json"), metadata, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "krasis.exe"), []byte("unit-test fixture; never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(path, "krasis.exe")
}

func TestCurrentUISelectsExactAgentBuildAndNeverLaunchesAnOldRecord(t *testing.T) {
	m := testManager(t)
	revision := strings.Repeat("a", 40)
	currentUIStage(t, m.path, strings.Repeat("1", 32), 22, revision)
	currentUIStage(t, m.path, strings.Repeat("2", 32), 21, strings.Repeat("b", 40))
	currentUIStage(t, m.path, "not-a-stage", 21, revision)
	want := currentUIStage(t, m.path, strings.Repeat("3", 32), 21, revision)
	verified, launched := 0, 0
	err := relaunchCurrentUI(context.Background(), m.path, 21, revision,
		func(*securefs.Directory) (updatebundle.Metadata, error) {
			verified++
			return updatebundle.Metadata{Generation: 21, Revision: revision}, nil
		},
		func(path string) error {
			launched++
			if path != want {
				t.Fatal("launched a mismatched or caller-selected path", path)
			}
			return nil
		})
	if err != nil || verified != 1 || launched != 1 {
		t.Fatal("matching UI was not verified and launched exactly once", err, verified, launched)
	}
}

func TestCurrentUIRejectsInvalidBundleCancellationAndLaunchFailure(t *testing.T) {
	for _, mode := range []string{"invalid", "mismatched", "cancelled", "launch failed"} {
		t.Run(mode, func(t *testing.T) {
			m := testManager(t)
			revision := strings.Repeat("a", 40)
			currentUIStage(t, m.path, strings.Repeat("1", 32), 21, revision)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			launched := 0
			failure := errors.New("synthetic failure")
			err := relaunchCurrentUI(ctx, m.path, 21, revision, func(*securefs.Directory) (updatebundle.Metadata, error) {
				if mode == "invalid" {
					return updatebundle.Metadata{}, failure
				}
				if mode == "cancelled" {
					cancel()
				}
				generation := 21
				if mode == "mismatched" {
					generation++
				}
				return updatebundle.Metadata{Generation: generation, Revision: revision}, nil
			}, func(string) error { launched++; return failure })
			wantLaunches := 0
			if mode == "launch failed" {
				wantLaunches = 1
			}
			if err == nil || launched != wantLaunches {
				t.Fatal("invalid or cancelled UI launched, or failure erased", mode, err, launched)
			}
		})
	}
}

func TestCurrentUIStageAndFilesStayPinnedThroughLaunch(t *testing.T) {
	m := testManager(t)
	revision := strings.Repeat("a", 40)
	exe := currentUIStage(t, m.path, strings.Repeat("1", 32), 21, revision)
	err := relaunchCurrentUI(context.Background(), m.path, 21, revision,
		func(*securefs.Directory) (updatebundle.Metadata, error) {
			return updatebundle.Metadata{Generation: 21, Revision: revision}, nil
		}, func(string) error {
			for _, file := range []string{exe, filepath.Join(filepath.Dir(exe), "BUILD.json")} {
				if err := os.WriteFile(file, []byte("changed"), 0600); err == nil {
					t.Fatal("launch bytes could change after verification", file)
				}
				if err := os.Remove(file); err == nil {
					t.Fatal("launch bytes could be replaced after verification", file)
				}
			}
			if err := os.Rename(filepath.Dir(exe), filepath.Dir(exe)+"-moved"); err == nil {
				t.Fatal("stage path could change during launch")
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCurrentUIFullBundleVerification(t *testing.T) {
	path := os.Getenv("KRASIS_WORKSPACE_UPDATE")
	if path == "" {
		t.Skip("packaging integration check only")
	}
	data, err := os.ReadFile(filepath.Join(path, "BUILD.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata updatebundle.Metadata
	if json.Unmarshal(data, &metadata) != nil {
		t.Fatal("invalid fixture metadata")
	}
	dir, err := securefs.OpenReadDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	launched := false
	matched, err := launchMatchingUIStage(context.Background(), dir, path, metadata.Generation, metadata.Revision, verifyCurrentUIBundle, func(executable string) error {
		if executable != filepath.Join(path, "krasis.exe") {
			t.Fatal("unexpected executable")
		}
		launched = true
		return nil
	})
	if err != nil || !matched || !launched {
		t.Fatal("real verified bundle failed UI launch policy", matched, launched, err)
	}
}
