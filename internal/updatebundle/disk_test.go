package updatebundle

import (
	"bytes"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestDiskBundleValidatesLayoutsAndTransfersStageOwnership(t *testing.T) {
	for _, names := range [][]string{legacyNames, currentNames, installerNames, selectedNames(installerNames, true, true)} {
		data := fixtureLayout(t, names, InstallerBundleGeneration)
		parent, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		var checked int
		bundle, err := readStaged(bytes.NewReader(data), int64(len(data)), parent, strings.Repeat("a", 32), func(_ string, source io.ReaderAt, revision string) error {
			checked++
			if _, ok := source.(*os.File); !ok || revision != strings.Repeat("a", 40) {
				t.Fatal("executable is not inspected on disk")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if checked != 3 && checked != 4 {
			t.Fatal("skipped executable validation", checked)
		}
		if err := bundle.Stage(parent, strings.Repeat("b", 32)); err != nil {
			t.Fatal(err)
		}
		if err := bundle.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := parent.Stat(strings.Repeat("b", 32) + "/BUILD.json"); err != nil {
			t.Fatal("Close removed transferred bundle", err)
		}
		parent.Close()
	}
}

func TestPreparedDiskStagePublishesOnceOrCleansUp(t *testing.T) {
	for _, mode := range []string{"publish", "discard", "target appeared", "tampered"} {
		t.Run(mode, func(t *testing.T) {
			parent, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			source, target := strings.Repeat("a", 32), strings.Repeat("b", 32)
			data := fixtureLayout(t, currentNames)
			bundle, err := readStaged(bytes.NewReader(data), int64(len(data)), parent, source, func(string, io.ReaderAt, string) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			defer bundle.Close()
			if mode == "tampered" {
				if err := parent.WriteFile(source+"/krasis.exe", []byte("modified"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			publish, err := bundle.PrepareStage(parent, target)
			if mode == "tampered" {
				if !errors.Is(err, ErrInvalid) || publish != nil {
					t.Fatal("preparation accepted altered files", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parent.Stat(target); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("preparation published before authorization", err)
			}
			switch mode {
			case "discard":
				if err := bundle.Close(); err != nil {
					t.Fatal(err)
				}
				if !errors.Is(publish(), ErrInvalid) {
					t.Fatal("discarded preparation published")
				}
			case "target appeared":
				if err := parent.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if !errors.Is(publish(), ErrInvalid) {
					t.Fatal("publication replaced a newly created target")
				}
			default:
				if err := publish(); err != nil {
					t.Fatal(err)
				}
				if !errors.Is(publish(), ErrInvalid) {
					t.Fatal("publication replay accepted")
				}
			}
			if err := bundle.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := parent.Stat(source); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("private staging directory retained", err)
			}
			if mode == "publish" {
				if _, err := parent.Stat(target + "/BUILD.json"); err != nil {
					t.Fatal("cleanup removed published files", err)
				}
			}
		})
	}
}

func TestDiskBundleRejectsTamperingAndCleansPrivateFiles(t *testing.T) {
	for _, scenario := range []string{"invalid_hash", "invalid_executable", "modified_stage", "extra_stage"} {
		t.Run(scenario, func(t *testing.T) {
			data := fixture(t, func(files map[string][]byte) {
				if scenario == "invalid_hash" {
					files["krasis.exe"] = []byte("modified")
				}
			}, "")
			parent, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			id := strings.Repeat("c", 32)
			bundle, err := readStaged(bytes.NewReader(data), int64(len(data)), parent, id, func(string, io.ReaderAt, string) error {
				if scenario == "invalid_executable" {
					return ErrInvalid
				}
				return nil
			})
			if strings.HasSuffix(scenario, "stage") {
				if err != nil {
					t.Fatal(err)
				}
				name := "krasis.exe"
				if scenario == "extra_stage" {
					name = "unknown.exe"
				}
				if err := parent.WriteFile(id+"/"+name, []byte("modified"), 0600); err != nil {
					t.Fatal(err)
				}
				if !errors.Is(bundle.Stage(parent, strings.Repeat("d", 32)), ErrInvalid) {
					t.Fatal("published altered stage")
				}
				if err := bundle.Close(); err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrInvalid) {
				t.Fatal("accepted invalid bundle", err)
			}
			if _, err := parent.Stat(id); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("private stage retained", err)
			}
		})
	}
}

func TestDiskBundleLargeExecutableDoesNotAllocateItsExpandedSize(t *testing.T) {
	data := fixture(t, func(files map[string][]byte) {
		// A bad checksum is deliberate: extraction must stay bounded even before
		// an untrusted executable can pass digest/provenance validation.
		files["krasis.exe"] = make([]byte, 16<<20)
	}, "")
	parent, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = readStaged(bytes.NewReader(data), int64(len(data)), parent, strings.Repeat("e", 32), func(string, io.ReaderAt, string) error { return nil })
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 4<<20 {
		t.Fatalf("allocated %d bytes while rejecting 16 MiB executable", allocated)
	}
}

func TestPackToPropagatesDestinationFailureAndBoundsArchive(t *testing.T) {
	data := fixtureLayout(t, currentNames)
	bundle, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	id := strings.Repeat("f", 32)
	if err := bundle.Stage(root, id); err != nil {
		t.Fatal(err)
	}
	source, err := root.OpenRoot(id)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	failure := errors.New("disk full")
	if !errors.Is(PackTo(source, failedArchiveWriter{failure}), failure) {
		t.Fatal("lost destination error")
	}
	limited := archiveLimitWriter{writer: io.Discard, written: MaxArchiveBytes - 1}
	if _, err := limited.Write([]byte("xx")); !errors.Is(err, ErrInvalid) {
		t.Fatal("archive limit bypassed")
	}
	var packed bytes.Buffer
	if err := PackTo(source, &packed); err != nil {
		t.Fatal(err)
	}
	if _, err := read(bytes.NewReader(packed.Bytes()), int64(packed.Len()), func(string, []byte, string) error { return nil }); err != nil {
		t.Fatal("streamed pack changed layout", err)
	}
}

type failedArchiveWriter struct{ err error }

func (w failedArchiveWriter) Write([]byte) (int, error) { return 0, w.err }
