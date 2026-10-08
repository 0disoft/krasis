package updatebundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, mutate func(map[string][]byte), extra string, withProbe ...bool) []byte {
	t.Helper()
	includeProbe := len(withProbe) != 0 && withProbe[0]
	selected := selectedNames(legacyNames, includeProbe, false)
	files := map[string][]byte{}
	for _, name := range selected {
		files[name] = []byte("test fixture")
	}
	files["BUILD.json"], _ = json.Marshal(Metadata{Revision: strings.Repeat("a", 40), Platform: "windows/amd64", Purpose: "manual-two-pc-smoke", Generation: 21})
	var sums strings.Builder
	for _, name := range selected[:len(selected)-1] {
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(files[name]), name)
	}
	files["SHA256SUMS.txt"] = []byte(sums.String())
	if mutate != nil {
		mutate(files)
	}
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for _, name := range selected {
		if data, ok := files[name]; ok {
			f, err := w.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			f.Write(data)
		}
	}
	if extra != "" {
		f, _ := w.Create(extra)
		f.Write([]byte("extra"))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestBundleAcceptsBothExclusiveExecutableLayouts(t *testing.T) {
	for _, current := range []bool{false, true} {
		selected := legacyNames
		ui := "krasis-control.exe"
		if current {
			selected = currentNames
			ui = "krasis.exe"
		}
		data := fixtureLayout(t, selected)
		bundle, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil })
		if err != nil || bundle.UIExecutable() != ui {
			t.Fatalf("current=%v: %v", current, err)
		}
		root, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := bundle.Stage(root, strings.Repeat("d", 32)); err != nil {
			t.Fatal(err)
		}
		root.Close()
	}
}

func TestReleaseMetadataDoesNotAcceptSignedClaimsInDevelopmentBundles(t *testing.T) {
	for _, tc := range []struct {
		purpose          string
		signed, accepted bool
	}{{"windows-release", true, true}, {"manual-two-pc-smoke", true, false}, {"windows-release", false, false}, {"untrusted", true, false}} {
		data := fixture(t, func(files map[string][]byte) {
			files["BUILD.json"], _ = json.Marshal(Metadata{Revision: strings.Repeat("a", 40), Platform: "windows/amd64", Generation: 234, Purpose: tc.purpose, Signed: tc.signed})
			var sums strings.Builder
			for _, name := range legacyNames[:len(legacyNames)-1] {
				fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(files[name]), name)
			}
			files["SHA256SUMS.txt"] = []byte(sums.String())
		}, "")
		_, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil })
		if (err == nil) != tc.accepted {
			t.Fatal(tc, err)
		}
	}
}

func TestSignedArchiveBytesMustMatchStagedDirectory(t *testing.T) {
	data := fixtureLayout(t, currentNames)
	b, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	parent, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	id := strings.Repeat("a", 32)
	if err = b.Stage(parent, id); err != nil {
		t.Fatal(err)
	}
	dir, err := parent.OpenRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if err = b.MatchDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if err = dir.WriteFile("krasis.exe", []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if b.MatchDirectory(dir) == nil {
		t.Fatal("tampered stage accepted")
	}
}

func fixtureLayout(t *testing.T, selected []string, generation ...int) []byte {
	t.Helper()
	files := map[string][]byte{}
	for _, name := range selected[:len(selected)-1] {
		files[name] = []byte("test fixture")
	}
	g := 76
	if len(generation) != 0 {
		g = generation[0]
	}
	files["BUILD.json"], _ = json.Marshal(Metadata{Revision: strings.Repeat("a", 40), Platform: "windows/amd64", Purpose: "manual-two-pc-smoke", Generation: g})
	var sums strings.Builder
	for _, name := range selected[:len(selected)-1] {
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(files[name]), name)
	}
	files["SHA256SUMS.txt"] = []byte(sums.String())
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for _, name := range selected {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestBundleRejectsMixedLayouts(t *testing.T) {
	for _, selected := range [][]string{
		{"krasis.exe", "krasis-agent.exe", "krasis-cli.exe", "krasis-control.exe", "START-HERE.md", "BUILD.json", "THIRD-PARTY-NOTICES.txt", "SHA256SUMS.txt"},
		{"krasis.exe", "krasis-agent.exe", "START-HERE.md", "BUILD.json", "THIRD-PARTY-NOTICES.txt", "SHA256SUMS.txt"},
	} {
		data := fixtureLayout(t, selected)
		if _, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil }); err == nil {
			t.Fatal("mixed or incomplete layout accepted")
		}
	}
}

func TestBundleRejectsUnsafeAndCorruptArchives(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		mutate      func(map[string][]byte)
	}{
		{name: "checksum", mutate: func(f map[string][]byte) { f["krasis.exe"] = []byte("tampered") }},
		{name: "missing", mutate: func(f map[string][]byte) { delete(f, "krasis.exe") }},
		{name: "traversal", extra: "../krasis.exe"},
		{name: "duplicate", extra: "krasis.exe"},
		{name: "case alias", extra: "KRASIS.EXE"},
		{name: "ADS", extra: "krasis.exe:other"},
		{name: "unknown", extra: "run.ps1"},
		{name: "large document", mutate: func(f map[string][]byte) { f["BUILD.json"] = bytes.Repeat([]byte("x"), 8193) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := fixture(t, tc.mutate, tc.extra)
			if _, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil }); err == nil {
				t.Fatal("accepted invalid bundle")
			}
		})
	}
}

func TestBundleStagesOnlyVerifiedFixedNamesWithoutOverwrite(t *testing.T) {
	data := fixture(t, nil, "")
	b, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	id := strings.Repeat("b", 32)
	if err := b.Stage(root, id); err != nil {
		t.Fatal(err)
	}
	if err := b.Stage(root, id); err == nil {
		t.Fatal("overwrote previous version")
	}
	if err := b.Stage(root, "../escape"); err == nil {
		t.Fatal("accepted arbitrary destination")
	}
	f, err := root.Open(id + "/krasis.exe")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := Read(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("production validator accepted fake executable")
	}
	if _, err := Read(bytes.NewReader(data), MaxArchiveBytes+1); err == nil {
		t.Fatal("accepted oversized archive")
	}
}

func TestProbePayloadStagesAsAnAtomicPair(t *testing.T) {
	data := fixture(t, nil, "", true)
	b, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	id := strings.Repeat("c", 32)
	if err := b.Stage(root, id); err != nil {
		t.Fatal(err)
	}
	for _, name := range probeNames {
		f, err := root.Open(id + "/" + name)
		if err != nil {
			t.Fatalf("missing staged %s: %v", name, err)
		}
		f.Close()
	}
	for _, tc := range []struct {
		name   string
		mutate func(map[string][]byte)
	}{
		{"missing package", func(f map[string][]byte) { delete(f, probeNames[0]) }},
		{"missing certificate", func(f map[string][]byte) { delete(f, probeNames[1]) }},
		{"oversized certificate", func(f map[string][]byte) { f[probeNames[1]] = bytes.Repeat([]byte("x"), (16<<10)+1) }},
		{"tampered package", func(f map[string][]byte) { f[probeNames[0]] = []byte("tampered") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := fixture(t, tc.mutate, "", true)
			if _, err := read(bytes.NewReader(invalid), int64(len(invalid)), func(string, []byte, string) error { return nil }); err == nil {
				t.Fatal("accepted invalid probe payload")
			}
		})
	}
}

func TestUIAccessPayloadStagesOnlyAsACompleteSet(t *testing.T) {
	for _, brokered := range []bool{false, true} {
		selected := selectedNames(currentNames, brokered, true)
		data := fixtureLayout(t, selected)
		bundle, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil })
		if err != nil {
			t.Fatalf("brokered=%t: %v", brokered, err)
		}
		root, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		id := strings.Repeat("d", 32)
		if err := bundle.Stage(root, id); err != nil {
			t.Fatal(err)
		}
		for _, name := range uiAccessProbeNames {
			file, err := root.Open(id + "/" + name)
			if err != nil {
				t.Fatalf("missing %s: %v", name, err)
			}
			file.Close()
		}
		root.Close()
	}
	for _, missing := range uiAccessProbeNames {
		files := selectedNames(currentNames, false, true)
		without := make([]string, 0, len(files)-1)
		for _, name := range files {
			if name != missing {
				without = append(without, name)
			}
		}
		data := fixtureLayout(t, without)
		if _, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil }); err == nil {
			t.Fatalf("accepted missing %s", missing)
		}
	}
}
