package peerupdate

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

// Previous preparation path, retained as an independent compatibility oracle
// and benchmark baseline. The random identifier is intentionally not compared.
func prepareClipboardLocalDoubleParse(local *ClipboardValue) (*ClipboardValue, error) {
	if local.Format == "" {
		if len(local.Data) != 0 || !validClipboardText(local.Text) {
			return nil, ErrDenied
		}
	} else if local.Text != "" || len(local.Data) == 0 || len(local.Data) > clipboardPayloadLimit(local.Format) {
		return nil, ErrDenied
	}
	value := &ClipboardValue{ID: randomID(), Text: local.Text, Format: local.Format, Data: bytes.Clone(local.Data)}
	if value.Format != "" && !ValidClipboardPayload(value.Format, value.Data) {
		return nil, ErrDenied
	}
	value.legacyFiles = value.Format == "files" && clipboardFilesFitLegacy(value.Data)
	return value, nil
}

func TestClipboardMetadataReusePreservesSizeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name          string
		sizes         []uint64
		valid, legacy bool
	}{
		{"empty file", []uint64{0}, true, true},
		{"before legacy limit", []uint64{legacyMaxClipboardFileBytes - 1}, true, true},
		{"at legacy limit", []uint64{legacyMaxClipboardFileBytes}, true, true},
		{"after legacy limit", []uint64{legacyMaxClipboardFileBytes + 1}, true, false},
		{"sum at legacy limit", []uint64{legacyMaxClipboardFileBytes - 1, 1}, true, true},
		{"sum after legacy limit", []uint64{legacyMaxClipboardFileBytes, 1}, true, false},
		{"before current limit", []uint64{MaxClipboardFileBytes - 1}, true, false},
		{"at current limit", []uint64{MaxClipboardFileBytes}, true, false},
		{"after current limit", []uint64{MaxClipboardFileBytes + 1}, false, false},
		{"overflow", []uint64{1, ^uint64(0)}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := clipboardSizeMetadataZip(t, tc.sizes...)
			local := &ClipboardValue{Format: "files", Data: data}
			before := bytes.Clone(data)
			want, oldErr := prepareClipboardLocalDoubleParse(local)
			got, err := prepareClipboardLocal(local)
			if tc.valid {
				if err != nil || oldErr != nil || got.legacyFiles != tc.legacy || got.legacyFiles != want.legacyFiles || !bytes.Equal(got.Data, want.Data) {
					t.Fatalf("legacy=%v result=%v want=%v errors=%v/%v", tc.legacy, got, want, err, oldErr)
				}
				got.Data[0] ^= 0xff // Returned bytes must still be privately owned.
			} else if !errors.Is(err, ErrDenied) || !errors.Is(oldErr, ErrDenied) || got != nil {
				t.Fatalf("invalid metadata accepted: %v / %v", err, oldErr)
			}
			if !bytes.Equal(local.Data, before) {
				t.Fatal("preparation shared or modified caller-owned bytes")
			}
		})
	}
}

func TestClipboardMetadataReusePreservesOrderAndDuplicateRejection(t *testing.T) {
	for index, names := range [][]string{
		{"folder/", "folder/file"},
		{"folder/file", "folder/"},
		{"folder/", "folder/file", "folder/FILE"},
		{"file", "file/child"},
		{"../file"},
		{"file", "file"},
	} {
		t.Run(strings.Join(names, ","), func(t *testing.T) {
			var out bytes.Buffer
			w := zip.NewWriter(&out)
			for _, name := range names {
				if _, err := w.CreateRaw(&zip.FileHeader{Name: name, Method: zip.Store}); err != nil {
					t.Fatal(err)
				}
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			local := &ClipboardValue{Format: "files", Data: out.Bytes()}
			want, oldErr := prepareClipboardLocalDoubleParse(local)
			got, err := prepareClipboardLocal(local)
			if (err == nil) != (index == 0) {
				t.Fatalf("directory order or duplicate policy changed: %v", err)
			}
			if !errors.Is(err, oldErr) || (got == nil) != (want == nil) {
				t.Fatalf("result changed: %v / %v", err, oldErr)
			}
			if got != nil && (!bytes.Equal(got.Data, want.Data) || got.legacyFiles != want.legacyFiles) {
				t.Fatal("entry order, content or compatibility changed")
			}
		})
	}
}

func BenchmarkPrepareClipboardFileMetadata(b *testing.B) {
	for _, count := range []int{1, MaxClipboardFiles} {
		data := clipboardEntryZIP(b, count, "")
		name := "one_file"
		if count == MaxClipboardFiles {
			name = "512_files"
		}
		for _, strategy := range []struct {
			name    string
			prepare func(*ClipboardValue) (*ClipboardValue, error)
		}{{"double_parse", prepareClipboardLocalDoubleParse}, {"reuse", prepareClipboardLocal}} {
			b.Run(name+"/"+strategy.name, func(b *testing.B) {
				local := &ClipboardValue{Format: "files", Data: data}
				b.ReportAllocs()
				for b.Loop() {
					value, err := strategy.prepare(local)
					if err != nil || !value.legacyFiles {
						b.Fatal("valid files rejected", err)
					}
				}
			})
		}
	}
}
