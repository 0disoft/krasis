package peerupdate

import (
	"archive/zip"
	"bytes"
	"path"
	"strings"
	"unicode/utf8"
)

const MaxClipboardFiles = 512
const MaxClipboardFileBytes = 64 << 20
const legacyMaxClipboardFileBytes = 40 << 20

func clipboardFilesFitLegacy(data []byte) bool {
	if len(data) == 0 || len(data) > legacyMaxClipboardFileBytes || !clipboardZIPWithinEntryLimit(data) {
		return false
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	var total uint64
	for _, f := range r.File {
		if f.UncompressedSize64 > uint64(legacyMaxClipboardFileBytes)-total {
			return false
		}
		total += f.UncompressedSize64
	}
	return true
}

// ValidClipboardFiles validates the metadata before a peer can advertise or
// receive a file copy. Extraction must separately enforce these byte limits.
func ValidClipboardFiles(data []byte) bool {
	if len(data) == 0 || len(data) > MaxClipboardFileBytes || !clipboardZIPWithinEntryLimit(data) {
		return false
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(r.File) == 0 || len(r.File) > MaxClipboardFiles {
		return false
	}
	seen := make(map[string]bool, len(r.File))
	var total uint64
	for _, f := range r.File {
		isDir := strings.HasSuffix(f.Name, "/")
		name := strings.TrimSuffix(f.Name, "/")
		if !validClipboardFilePath(name) || f.Flags&1 != 0 ||
			(isDir && f.UncompressedSize64 != 0) ||
			(!isDir && !f.Mode().IsRegular()) ||
			(isDir && !f.Mode().IsDir()) {
			return false
		}
		key := strings.ToUpper(name)
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = isDir
		if parent := path.Dir(name); parent != "." && !seen[strings.ToUpper(parent)] {
			return false // Directory entries must precede their contents.
		}
		// Check the remaining budget before adding untrusted ZIP64 sizes.
		// Addition first could wrap uint64 and admit an oversized archive.
		if f.UncompressedSize64 > uint64(MaxClipboardFileBytes)-total {
			return false
		}
		total += f.UncompressedSize64
	}
	return true
}

func validClipboardFilePath(name string) bool {
	if name == "" || len(name) > 1000 || !utf8.ValidString(name) || path.Clean(name) != name {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || len(part) > 240 ||
			strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") ||
			strings.ContainsAny(part, "<>:\"\\|?*") {
			return false
		}
		for _, ch := range part {
			if ch < 32 {
				return false
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
			len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return false
		}
	}
	return true
}

func ValidClipboardPayload(format string, data []byte) bool {
	if format == "files" {
		return ValidClipboardFiles(data)
	}
	return ValidClipboardImage(format, data)
}
