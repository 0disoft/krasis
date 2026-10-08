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
	"strings"
	"testing"
)

func TestInstallPreflightDoesNotCloseProductOnBadReceiptSourceOrCancellation(t *testing.T) {
	for _, scenario := range []string{"malformed", "oversized", "newer", "folder_limit", "missing_source", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			base := t.TempDir()
			dir, _, err := directory(base)
			if err != nil {
				t.Fatal(err)
			}
			defer dir.Close()
			source := filepath.Join(t.TempDir(), "setup.exe")
			if err := os.WriteFile(source, []byte("source bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			next := Receipt{Schema: Schema, Generation: 251, Revision: strings.Repeat("a", 40), Folders: []string{strings.Repeat("b", 32)}}
			ctx := context.Background()
			var previous []byte
			switch scenario {
			case "malformed":
				previous = []byte("{}")
			case "oversized":
				previous = []byte(strings.Repeat(" ", 8193))
			case "newer":
				old := next
				old.Generation = 252
				previous, _ = json.Marshal(old)
			case "folder_limit":
				old := next
				old.Folders = nil
				for i := range 64 {
					old.Folders = append(old.Folders, fmt.Sprintf("%032x", i))
				}
				previous, _ = json.Marshal(old)
			case "missing_source":
				source = filepath.Join(filepath.Dir(source), "absent.exe")
			case "cancelled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			if previous != nil {
				if err := replace(dir, "installation.json", previous); err != nil {
					t.Fatal(err)
				}
			}
			stops := 0
			_, err = prepareInstallPlan(ctx, dir, source, next, func() (io.Closer, error) { stops++; return &shutdownLease{}, nil })
			if err == nil || stops != 0 {
				t.Fatal("preflight stopped usable product", scenario, err, stops)
			}
			if previous != nil {
				data, e := read(dir, "installation.json", 8193)
				if e != nil || string(data) != string(previous) {
					t.Fatal("receipt was changed", e)
				}
			}
		})
	}
}

func TestInstallUsesPreflightSourceSnapshotAndReleasesCancelledShutdownLease(t *testing.T) {
	for _, cancelDuringStop := range []bool{false, true} {
		dir, _, err := directory(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		source := filepath.Join(t.TempDir(), "setup.exe")
		if err := os.WriteFile(source, []byte("verified preflight bytes"), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		lease := &shutdownLease{}
		next := Receipt{Schema: Schema, Generation: 251, Revision: strings.Repeat("a", 40), Folders: []string{strings.Repeat("b", 32)}}
		plan, err := prepareInstallPlan(ctx, dir, source, next, func() (io.Closer, error) {
			if e := os.Remove(source); e != nil {
				t.Fatal(e)
			}
			if cancelDuringStop {
				cancel()
			}
			return lease, nil
		})
		if cancelDuringStop {
			if !errors.Is(err, context.Canceled) || !lease.closed {
				t.Fatal("cancelled shutdown lease leaked", err)
			}
		} else {
			if err != nil || string(plan.setup) != "verified preflight bytes" || plan.idle != lease || lease.closed {
				t.Fatal("preflight snapshot discarded", plan, err)
			}
			plan.idle.Close()
		}
		cancel()
		dir.Close()
	}
}
