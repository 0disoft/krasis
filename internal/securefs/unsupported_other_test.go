//go:build !windows && !linux

package securefs

import (
	"errors"
	"testing"
)

func TestUnsupportedPlatformFailsClosed(t *testing.T) {
	if d, err := OpenDirectory(t.TempDir()); d != nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("OpenDirectory = %v, %v", d, err)
	}
	if d, err := OpenReadDirectory(t.TempDir()); d != nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("OpenReadDirectory = %v, %v", d, err)
	}
	d := &Directory{}
	checks := []func() error{
		func() error { _, err := d.Open("file"); return err },
		func() error { _, err := d.OpenObserved("file", nil); return err },
		func() error { _, err := d.OpenReadWrite("file"); return err },
		func() error { _, err := d.OpenLock("file", true); return err },
		func() error { _, err := d.CreateLock("file", 0o600); return err },
		func() error { _, err := d.CreateExclusive("file", 0o600); return err },
		func() error { _, _, err := d.CreateTemporary("tmp", 0o600); return err },
		func() error { _, err := d.CreateDirectory("dir", 0o700); return err },
		func() error { return d.Replace("file", "target") },
		func() error { return d.PromoteNoReplace("file", "target") },
		func() error { return d.Remove("file") },
		func() error { return d.RemoveSelf() },
		func() error { _, err := d.ReadNames(); return err },
	}
	for i, check := range checks {
		if err := check(); !errors.Is(err, ErrUnsupportedPlatform) {
			t.Errorf("operation %d: %v", i, err)
		}
	}
}
