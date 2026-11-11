//go:build linux

package residentagent

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Each scenario keeps its own executable paths and transient unit namespace.
// A missing/invalid prepared binary is a failure, never a fallback rebuild while
// other agents are capturing input. Direct go test still builds from source.
func linuxX11CopyBinaries(source, destination string, actual bool) error {
	if !filepath.IsAbs(source) {
		return errors.New("prepared X11 binary directory must be absolute")
	}
	names := []string{"krasis-agent"}
	if actual {
		names = append(names, "krasis-cli")
	}
	for _, name := range names {
		if err := linuxX11CopyBinary(filepath.Join(source, name), filepath.Join(destination, name)); err != nil {
			return err
		}
	}
	return nil
}

func linuxX11CopyBinary(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("prepared X11 binary must be a regular executable")
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}

func TestLinuxX11PreparedBinaries(t *testing.T) {
	source := t.TempDir()
	for _, name := range []string{"krasis-agent", "krasis-cli"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	first, second := t.TempDir(), t.TempDir()
	for _, destination := range []string{first, second} {
		if err := linuxX11CopyBinaries(source, destination, true); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"krasis-agent", "krasis-cli"} {
			data, err := os.ReadFile(filepath.Join(destination, name))
			if err != nil || string(data) != name {
				t.Fatalf("prepared %s: %q %v", name, data, err)
			}
		}
	}
	if err := linuxX11CopyBinaries(source, first, true); err == nil {
		t.Fatal("overwrote an existing scenario binary")
	}
	if err := linuxX11CopyBinaries("relative", t.TempDir(), true); err == nil {
		t.Fatal("accepted a relative executable directory")
	}
	if err := os.Remove(filepath.Join(source, "krasis-cli")); err != nil {
		t.Fatal(err)
	}
	if err := linuxX11CopyBinaries(source, t.TempDir(), true); err == nil {
		t.Fatal("accepted a missing CLI")
	}
	if err := os.Chmod(filepath.Join(source, "krasis-agent"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := linuxX11CopyBinaries(source, t.TempDir(), false); err == nil {
		t.Fatal("accepted a non-executable agent")
	}
}
