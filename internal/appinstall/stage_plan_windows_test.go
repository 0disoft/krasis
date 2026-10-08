//go:build windows

package appinstall

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestImagePreparationFailureLeavesUsableProductAndReceiptUntouched(t *testing.T) {
	for _, scenario := range []string{"disk_full", "verification", "cancelled", "ready"} {
		t.Run(scenario, func(t *testing.T) {
			dir, _, err := directory(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer dir.Close()
			source := filepath.Join(t.TempDir(), "setup.exe")
			if err := os.WriteFile(source, []byte("preflight source snapshot"), 0600); err != nil {
				t.Fatal(err)
			}
			old := Receipt{Schema: Schema, Generation: 254, Revision: strings.Repeat("a", 40), Folders: []string{strings.Repeat("b", 32)}}
			body, _ := json.Marshal(old)
			if err := replace(dir, "installation.json", body); err != nil {
				t.Fatal(err)
			}
			next := Receipt{Schema: Schema, Generation: 255, Revision: strings.Repeat("c", 40), Folders: []string{strings.Repeat("d", 32)}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			prepared, stops := false, 0
			plan, err := prepareInstallPlan(ctx, dir, source, next, nil, func(r Receipt) error {
				if r.Folders[0] != next.Folders[0] || len(r.Folders) != 2 {
					t.Fatal("image prepared outside merged install plan", r)
				}
				switch scenario {
				case "disk_full":
					return windows.ERROR_DISK_FULL
				case "verification":
					return installationFailure("verify_image", ErrInvalid)
				case "cancelled":
					cancel()
				}
				prepared = true
				return nil
			}, func() (io.Closer, error) {
				if !prepared {
					t.Fatal("product stopped before image preparation")
				}
				stops++
				return &shutdownLease{}, nil
			})
			if scenario == "ready" {
				if err != nil || stops != 1 || string(plan.setup) != "preflight source snapshot" {
					t.Fatal("prepared image did not proceed to shutdown", stops, err)
				}
				plan.idle.Close()
			} else {
				want, stage := error(windows.ERROR_DISK_FULL), "stage_image"
				if scenario == "verification" {
					want, stage = ErrInvalid, "verify_image"
				} else if scenario == "cancelled" {
					want = context.Canceled
				}
				if !errors.Is(err, want) || InstallationStage(err) != stage || stops != 0 {
					t.Fatal("preparation failure interrupted product or lost cause", stops, err)
				}
			}
			got, err := read(dir, "installation.json", 8192)
			if err != nil || string(got) != string(body) {
				t.Fatal("image preparation published installation receipt", err)
			}
		})
	}
}
