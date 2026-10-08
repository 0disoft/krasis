//go:build windows

package appinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRepairPlanReservesFreshFolderAndRetainsPreviousHistory(t *testing.T) {
	original, fresh := strings.Repeat("a", 32), strings.Repeat("b", 32)
	next := Receipt{Schema: Schema, Generation: 252, Revision: strings.Repeat("c", 40), Folders: []string{original, strings.Repeat("d", 32)}}
	want := append([]string{fresh}, next.Folders...)
	got, err := selectInstallFolder(next, installFolderOps{
		match: func(id string) error {
			if id == original {
				return errDamagedInstallFolder
			}
			return os.ErrNotExist
		},
		newID: func() (string, error) { return fresh, nil },
	})
	if err != nil || !reflect.DeepEqual(got.Folders, want) || next.Folders[0] != original {
		t.Fatal("repair lost existing folders", got, err)
	}
}

func TestFolderPlanDoesNotRepairHealthyMissingOrUnreadableImage(t *testing.T) {
	for _, cause := range []error{nil, os.ErrNotExist, os.ErrPermission} {
		next := Receipt{Folders: []string{strings.Repeat("a", 32)}}
		generated := 0
		got, err := selectInstallFolder(next, installFolderOps{
			match: func(string) error { return cause }, newID: func() (string, error) { generated++; return strings.Repeat("b", 32), nil },
		})
		want := cause
		if errors.Is(cause, os.ErrNotExist) {
			want = nil
		}
		if generated != 0 || !errors.Is(err, want) {
			t.Fatal("inspection was treated as damage", cause, err, generated)
		}
		if cause == nil || errors.Is(cause, os.ErrNotExist) {
			if !reflect.DeepEqual(got.Folders, next.Folders) {
				t.Fatal(got)
			}
		}
	}
}

func TestRepairPreflightAtHistoryLimitLeavesProductAndFilesUntouched(t *testing.T) {
	dir, path, err := directory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	next := Receipt{Schema: Schema, Generation: 252, Revision: strings.Repeat("c", 40)}
	for i := range 64 {
		next.Folders = append(next.Folders, fmt.Sprintf("%032x", i))
	}
	data, _ := json.Marshal(next)
	if err := replace(dir, "installation.json", data); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "setup.exe")
	if err := os.WriteFile(source, []byte("setup source"), 0600); err != nil {
		t.Fatal(err)
	}
	stops, generated := 0, 0
	_, err = prepareInstallPlan(context.Background(), dir, source, Receipt{Schema: Schema, Generation: 252, Revision: next.Revision, Folders: next.Folders[:1]},
		func(r Receipt) (Receipt, error) {
			return selectInstallFolder(r, installFolderOps{
				match: func(string) error { return errDamagedInstallFolder }, newID: func() (string, error) { generated++; return strings.Repeat("d", 32), nil },
			})
		}, func() (io.Closer, error) { stops++; return &shutdownLease{}, nil })
	if !errors.Is(err, ErrInvalid) || stops != 0 || generated != 0 {
		t.Fatal("exhausted repair interrupted product", err, stops, generated)
	}
	names, err := os.ReadDir(path)
	if err != nil || len(names) != 1 || names[0].Name() != "installation.json" {
		t.Fatal("preflight created an orphan", names, err)
	}
	got, err := read(dir, "installation.json", 8192)
	if err != nil || string(got) != string(data) {
		t.Fatal("preflight changed receipt", err)
	}
}

func TestRepairPlanRejectsCollisionBadIDAndEntropyFailure(t *testing.T) {
	original, fresh := strings.Repeat("a", 32), strings.Repeat("b", 32)
	for _, scenario := range []string{"collision", "existing_image", "bad_id", "entropy", "unreadable_candidate"} {
		t.Run(scenario, func(t *testing.T) {
			_, err := selectInstallFolder(Receipt{Folders: []string{original}}, installFolderOps{
				match: func(id string) error {
					if id == original {
						return errDamagedInstallFolder
					}
					if scenario == "existing_image" {
						return nil
					}
					if scenario == "unreadable_candidate" {
						return os.ErrPermission
					}
					return os.ErrNotExist
				},
				newID: func() (string, error) {
					switch scenario {
					case "collision":
						return original, nil
					case "bad_id":
						return "../identity", nil
					case "entropy":
						return "", io.ErrUnexpectedEOF
					}
					return fresh, nil
				},
			})
			if err == nil {
				t.Fatal("unknown replacement accepted")
			}
		})
	}
}
