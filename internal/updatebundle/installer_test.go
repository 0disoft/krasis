package updatebundle

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestInstallerBundleVerifiesAndStagesWithoutChangingUIEntryPoint(t *testing.T) {
	for _, probe := range []bool{false, true} {
		data := fixtureLayout(t, selectedNames(installerNames, probe, probe), InstallerBundleGeneration)
		var verified []string
		bundle, err := read(bytes.NewReader(data), int64(len(data)), func(name string, _ []byte, revision string) error {
			if revision != strings.Repeat("a", 40) {
				t.Fatal("wrong build binding")
			}
			verified = append(verified, name)
			return nil
		})
		if err != nil || !bundle.HasInputInstaller() || bundle.UIExecutable() != "krasis.exe" || len(verified) != 4 || verified[3] != installerName {
			t.Fatal("installer changed UI launch or bypassed executable validation", err)
		}
		parent, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		id := strings.Repeat("d", 32)
		if err := bundle.Stage(parent, id); err != nil {
			t.Fatal(err)
		}
		root, err := parent.OpenRoot(id)
		if err != nil {
			t.Fatal(err)
		}
		file, err := root.Open(installerName)
		if err != nil {
			t.Fatal("installer was not staged")
		}
		file.Close()
		root.Close()
		parent.Close()
	}
}

func TestInstallerCannotEnterLegacyOrPreBridgeBundle(t *testing.T) {
	legacyInstaller := append(append([]string{}, legacyNames[:3]...), append([]string{installerName}, legacyNames[3:]...)...)
	for _, example := range []struct {
		names      []string
		generation int
	}{
		{legacyInstaller, InstallerBundleGeneration}, {installerNames, InstallerLayoutGeneration - 1},
	} {
		data := fixtureLayout(t, example.names, example.generation)
		if _, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil }); !errors.Is(err, ErrInvalid) {
			t.Fatal("unsupported installer layout accepted")
		}
	}
	data := fixtureLayout(t, installerNames, InstallerBundleGeneration)
	if _, err := read(bytes.NewReader(data), int64(len(data)), func(name string, _ []byte, _ string) error {
		if name == installerName {
			return ErrInvalid
		}
		return nil
	}); !errors.Is(err, ErrInvalid) {
		t.Fatal("installer executable failure ignored")
	}
}

func TestInstallerIsNotInferredForOrdinaryBundles(t *testing.T) {
	data := fixtureLayout(t, currentNames, InstallerLayoutGeneration)
	bundle, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil })
	if err != nil || bundle.HasInputInstaller() || (*Bundle)(nil).HasInputInstaller() {
		t.Fatal("ordinary bundle acquired installer")
	}
}
