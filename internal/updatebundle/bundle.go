// Package updatebundle validates manual-test bundles without executing them.
// Checksums establish integrity, not publisher authenticity or permission to run.
package updatebundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const MaxArchiveBytes int64 = 96 << 20
const MaxExpandedBytes int64 = 192 << 20
const maxExecutableBytes int64 = 60 << 20
const maxDocumentBytes int64 = 1 << 20

var ErrInvalid = errors.New("invalid Krasis update bundle")

var names = []string{"krasis.exe", "krasis-agent.exe", "krasis-control.exe", "START-HERE.md", "BUILD.json", "THIRD-PARTY-NOTICES.txt", "SHA256SUMS.txt"}

type Metadata struct {
	Revision   string `json:"revision"`
	Platform   string `json:"platform"`
	CGO        bool   `json:"cgo"`
	Purpose    string `json:"purpose"`
	Signed     bool   `json:"signed"`
	Generation int    `json:"generation"`
}

// Bundle owns bounded, verified bytes so staging cannot race a changed archive.
type Bundle struct {
	metadata Metadata
	files    map[string][]byte
}

func (b *Bundle) Metadata() Metadata { return b.metadata }

func Read(source io.ReaderAt, size int64) (*Bundle, error) {
	return read(source, size, verifyExecutable)
}

func read(source io.ReaderAt, size int64, executable func(string, []byte, string) error) (*Bundle, error) {
	if source == nil || size <= 0 || size > MaxArchiveBytes {
		return nil, ErrInvalid
	}
	r, err := zip.NewReader(source, size)
	if err != nil || len(r.File) != len(names) {
		return nil, ErrInvalid
	}
	files := make(map[string][]byte, len(names))
	var total int64
	for _, entry := range r.File {
		limit := fileLimit(entry.Name)
		if limit == 0 || entry.Mode()&os.ModeType != 0 || entry.Flags&1 != 0 || entry.UncompressedSize64 > uint64(limit) || files[entry.Name] != nil {
			return nil, ErrInvalid
		}
		stream, err := entry.Open()
		if err != nil {
			return nil, ErrInvalid
		}
		data, readErr := io.ReadAll(io.LimitReader(stream, limit+1))
		closeErr := stream.Close()
		total += int64(len(data))
		if readErr != nil || closeErr != nil || int64(len(data)) > limit || total > MaxExpandedBytes || uint64(len(data)) != entry.UncompressedSize64 {
			return nil, ErrInvalid
		}
		files[entry.Name] = data
	}
	for _, name := range names {
		if _, ok := files[name]; !ok {
			return nil, ErrInvalid
		}
	}
	checksums := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(files["SHA256SUMS.txt"]), "\n"), "\n") {
		parts := strings.SplitN(strings.TrimSuffix(line, "\r"), "  ", 2)
		if len(parts) != 2 || !validHex(parts[0], 64) || fileLimit(parts[1]) == 0 || parts[1] == "SHA256SUMS.txt" || checksums[parts[1]] != "" {
			return nil, ErrInvalid
		}
		checksums[parts[1]] = parts[0]
	}
	if len(checksums) != len(names)-1 {
		return nil, ErrInvalid
	}
	for name, expected := range checksums {
		hash := sha256.Sum256(files[name])
		if hex.EncodeToString(hash[:]) != expected {
			return nil, ErrInvalid
		}
	}
	var metadata Metadata
	decoder := json.NewDecoder(bytes.NewReader(files["BUILD.json"]))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&metadata) != nil || decoder.Decode(new(any)) != io.EOF || !validHex(metadata.Revision, 40) || metadata.Platform != "windows/amd64" || metadata.CGO || metadata.Purpose != "manual-two-pc-smoke" || metadata.Signed || metadata.Generation < 21 {
		return nil, ErrInvalid
	}
	for _, name := range names[:3] {
		if err := executable(name, files[name], metadata.Revision); err != nil {
			return nil, errors.Join(ErrInvalid, err)
		}
	}
	return &Bundle{metadata: metadata, files: files}, nil
}

func fileLimit(name string) int64 {
	for index, allowed := range names {
		if name == allowed {
			if index < 3 {
				return maxExecutableBytes
			}
			if name == "SHA256SUMS.txt" || name == "BUILD.json" {
				return 8192
			}
			return maxDocumentBytes
		}
	}
	return 0
}

func validHex(value string, length int) bool {
	if len(value) != length || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func verifyExecutable(name string, data []byte, revision string) error {
	file, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return ErrInvalid
	}
	defer file.Close()
	if file.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return ErrInvalid
	}
	info, err := buildinfo.Read(bytes.NewReader(data))
	if err != nil || info.Main.Path != "github.com/0disoft/krasis" || info.Path != "github.com/0disoft/krasis/cmd/"+strings.TrimSuffix(name, ".exe") {
		return ErrInvalid
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["GOOS"] != "windows" || settings["GOARCH"] != "amd64" || settings["CGO_ENABLED"] != "0" || settings["vcs.revision"] != revision || settings["vcs.modified"] != "false" {
		return ErrInvalid
	}
	return nil
}

// Stage creates a fresh directory under a caller-owned, protected root. It never
// overwrites an installed bundle and never uses archive-controlled path names.
func (b *Bundle) Stage(parent *os.Root, directory string) (err error) {
	if b == nil || len(b.files) != len(names) || !validHex(directory, 32) {
		return ErrInvalid
	}
	if err = parent.Mkdir(directory, 0700); err != nil {
		return err
	}
	root, err := parent.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	defer func() {
		if err != nil {
			for _, name := range names {
				_ = root.Remove(name)
			}
			_ = root.Close()
			_ = parent.Remove(directory)
		}
	}()
	for _, name := range names {
		file, createErr := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if createErr != nil {
			return createErr
		}
		_, writeErr := file.Write(b.files[name])
		syncErr := file.Sync()
		closeErr := file.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return err
		}
	}
	return nil
}

// Pack snapshots the current distribution. The returned ZIP is checked by Read
// before callers offer it; missing/mixed files do not become an update.
func Pack(root interface {
	Open(string) (*os.File, error)
}) ([]byte, error) {
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for _, name := range names {
		f, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > fileLimit(name) {
			f.Close()
			return nil, ErrInvalid
		}
		data, err := io.ReadAll(io.LimitReader(f, fileLimit(name)+1))
		closeErr := f.Close()
		if err != nil || closeErr != nil || int64(len(data)) > fileLimit(name) {
			return nil, ErrInvalid
		}
		entry, err := w.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	if _, err := Read(bytes.NewReader(buffer.Bytes()), int64(buffer.Len())); err != nil {
		return nil, fmt.Errorf("verify local distribution: %w", err)
	}
	return buffer.Bytes(), nil
}
