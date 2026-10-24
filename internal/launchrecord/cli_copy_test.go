package launchrecord

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
)

func TestCopyCLIComparisonAndReplacement(t *testing.T) {
	for _, tc := range []struct {
		name string
		old  []byte
		want []byte
	}{
		{"missing", nil, []byte("MZ-new")},
		{"unchanged", []byte("MZ-same"), []byte("MZ-same")},
		{"changed-same-size", []byte("MZ-old"), []byte("MZ-new")},
		{"changed-size", []byte("MZ-old"), []byte("MZ-new-larger")},
		{"invalid-destination", []byte{}, []byte("MZ-new")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, source := t.TempDir(), t.TempDir()
			destination, err := productidentity.PrepareLaunch(base)
			if err != nil {
				t.Fatal(err)
			}
			defer destination.Close()
			path, err := Directory(base)
			if err != nil {
				t.Fatal(err)
			}
			cli := filepath.Join(path, "krasis-cli.exe")
			stamp := time.Unix(123456, 0)
			if tc.old != nil {
				if err := replace(destination, "krasis-cli.exe", tc.old); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(cli, stamp, stamp); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(source, "krasis-cli.exe"), tc.want, 0600); err != nil {
				t.Fatal(err)
			}
			if err := copyCLI(destination, source); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(cli)
			if err != nil || !bytes.Equal(got, tc.want) {
				t.Fatalf("CLI = %q, %v; want %q", got, err, tc.want)
			}
			if bytes.Equal(tc.old, tc.want) {
				info, err := os.Stat(cli)
				if err != nil || !info.ModTime().Equal(stamp) {
					t.Fatalf("unchanged CLI was replaced: %v", err)
				}
			}
			file, err := destination.Open("krasis-cli.exe")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := peercredentials.ValidateSecurityFileHandle(file); err != nil {
				t.Fatal(err)
			}
			if err := destination.Close(); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".launch-") {
					t.Fatalf("temporary file remains: %s", entry.Name())
				}
			}
		})
	}
}

func TestCopyCLIRejectsInvalidSourceWithoutReplacingDestination(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int64
	}{{"empty", 0}, {"oversized", maxExecutable + 1}} {
		t.Run(tc.name, func(t *testing.T) {
			base, source := t.TempDir(), t.TempDir()
			destination, err := productidentity.PrepareLaunch(base)
			if err != nil {
				t.Fatal(err)
			}
			defer destination.Close()
			original := []byte("MZ-keep")
			if err := replace(destination, "krasis-cli.exe", original); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(filepath.Join(source, "krasis-cli.exe"))
			if err != nil {
				t.Fatal(err)
			}
			truncateErr := file.Truncate(tc.size)
			closeErr := file.Close()
			if truncateErr != nil || closeErr != nil {
				t.Fatalf("prepare source: %v, %v", truncateErr, closeErr)
			}
			if err := copyCLI(destination, source); !errors.Is(err, ErrInvalid) {
				t.Fatalf("copyCLI = %v; want ErrInvalid", err)
			}
			path, err := Directory(base)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(path, "krasis-cli.exe"))
			if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("destination changed after invalid source: %q, %v", got, err)
			}
		})
	}
}
