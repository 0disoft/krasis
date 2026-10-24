package peerupdate

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

func clipboardEntryZIP(t testing.TB, count int, comment string) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	if err := w.SetComment(comment); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		if _, err := w.CreateRaw(&zip.FileHeader{Name: fmt.Sprintf("file%d", i), Method: zip.Store}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestClipboardZIPEntryLimit(t *testing.T) {
	for _, count := range []int{0, 1, MaxClipboardFiles, MaxClipboardFiles + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			data := clipboardEntryZIP(t, count, "")
			want := count > 0 && count <= MaxClipboardFiles
			if got := clipboardZIPWithinEntryLimit(data); got != want {
				t.Fatalf("preflight accepted=%t, want %t", got, want)
			}
			if ValidClipboardFiles(data) != want || clipboardFilesFitLegacy(data) != want {
				t.Fatal("file admission bypassed entry limit")
			}
		})
	}
}

func TestClipboardZIPEntryLimitRejectsWrappedCountWithoutAllocating(t *testing.T) {
	data := clipboardEntryZIP(t, 65537, "")
	end := len(data) - 22
	// archive/zip ignores the ZIP64 end record when these fields do not use
	// sentinels, and accepts the actual count modulo 65536 as the claimed 1.
	binary.LittleEndian.PutUint16(data[end+8:], 1)
	binary.LittleEndian.PutUint16(data[end+10:], 1)
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) != 65537 {
		t.Fatalf("wrapped-count fixture: %v", err)
	}
	allocs := testing.AllocsPerRun(10, func() {
		if clipboardZIPWithinEntryLimit(data) {
			t.Fatal("wrapped count passed preflight")
		}
	})
	if allocs != 0 {
		t.Fatalf("preflight allocated %g objects", allocs)
	}
}

func TestClipboardZIPEntryLimitPreservesCommentsPrefixesAndZIP64(t *testing.T) {
	data := clipboardEntryZIP(t, 2, strings.Repeat("comment", 9000))
	for _, prefix := range []string{"", "self-extracting prefix"} {
		candidate := append([]byte(prefix), data...)
		if !ValidClipboardFiles(candidate) {
			t.Fatal("valid comment or prefix rejected")
		}
	}
	data = clipboardEntryZIP(t, 1, "")
	end := len(data) - 22
	var record [76]byte // ZIP64 end record followed by its locator.
	binary.LittleEndian.PutUint32(record[:], 0x06064b50)
	binary.LittleEndian.PutUint64(record[4:], 44)
	binary.LittleEndian.PutUint64(record[24:], 1)
	binary.LittleEndian.PutUint64(record[32:], 1)
	binary.LittleEndian.PutUint64(record[40:], uint64(binary.LittleEndian.Uint32(data[end+12:])))
	binary.LittleEndian.PutUint64(record[48:], uint64(binary.LittleEndian.Uint32(data[end+16:])))
	binary.LittleEndian.PutUint32(record[56:], 0x07064b50)
	binary.LittleEndian.PutUint64(record[64:], uint64(end))
	binary.LittleEndian.PutUint32(record[72:], 1)
	zip64 := append(append(append([]byte(nil), data[:end]...), record[:]...), data[end:]...)
	binary.LittleEndian.PutUint16(zip64[len(zip64)-12:], 0xffff)
	if !ValidClipboardFiles(zip64) {
		t.Fatal("valid ZIP64 directory rejected")
	}
	for _, offset := range []uint64{^uint64(0), uint64(len(zip64))} {
		broken := bytes.Clone(zip64)
		binary.LittleEndian.PutUint64(broken[len(broken)-34:], offset)
		if clipboardZIPWithinEntryLimit(broken) {
			t.Fatal("out-of-bounds ZIP64 locator accepted")
		}
	}
}

func BenchmarkClipboardZIPOversizedDirectory(b *testing.B) {
	data := clipboardEntryZIP(b, 65537, "")
	end := len(data) - 22
	binary.LittleEndian.PutUint16(data[end+8:], 1)
	binary.LittleEndian.PutUint16(data[end+10:], 1)
	for _, preflight := range []bool{false, true} {
		b.Run(fmt.Sprintf("preflight=%t", preflight), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if preflight {
					if ValidClipboardFiles(data) {
						b.Fatal("oversized directory accepted")
					}
					continue
				}
				reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
				if err == nil && len(reader.File) <= MaxClipboardFiles {
					b.Fatal("oversized directory accepted")
				}
			}
		})
	}
}

func FuzzClipboardZIPEntryLimit(f *testing.F) {
	f.Add([]byte(nil))
	f.Add(clipboardEntryZIP(f, 1, "comment"))
	f.Add(clipboardEntryZIP(f, MaxClipboardFiles, ""))
	f.Add(clipboardEntryZIP(f, MaxClipboardFiles+1, ""))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 || !clipboardZIPWithinEntryLimit(data) {
			return
		}
		reader, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if reader != nil && len(reader.File) > MaxClipboardFiles {
			t.Fatal("preflight allowed an oversized directory")
		}
	})
}
