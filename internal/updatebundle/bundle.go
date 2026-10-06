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

// g105 learns the installer layout without carrying it to pre-g105 receivers.
const InstallerLayoutGeneration = 105
const InstallerBundleGeneration = 106
const installerName = "krasis-install.exe"

var legacyNames = []string{"krasis.exe", "krasis-agent.exe", "krasis-control.exe", "START-HERE.md", "BUILD.json", "THIRD-PARTY-NOTICES.txt", "SHA256SUMS.txt"}
var currentNames = []string{"krasis.exe", "krasis-agent.exe", "krasis-cli.exe", "START-HERE.md", "BUILD.json", "THIRD-PARTY-NOTICES.txt", "SHA256SUMS.txt"}
var installerNames = append(append(append([]string{}, currentNames[:3]...), installerName), currentNames[3:]...)
var probeNames = []string{"KrasisBrokeredProbe-TESTSIGNED.msix", "KrasisBrokeredProbe-TESTCERT.cer"}
var uiAccessProbeNames = []string{"KrasisUIAccessProbe-TESTSIGNED.exe", "KrasisUIAccessProbe-TESTCERT.cer", "TEST-CERT-DETAILS.txt", "README.md", "install-test.ps1", "remove-test.ps1"}

func selectedNames(base []string, withProbe, withUIAccess bool) []string {
	selected := append([]string{}, base[:len(base)-1]...)
	if withProbe {
		selected = append(selected, probeNames...)
	}
	if withUIAccess {
		selected = append(selected, uiAccessProbeNames...)
	}
	return append(selected, base[len(base)-1])
}

func layout(files map[string][]byte) ([]string, error) {
	_, legacy := files["krasis-control.exe"]
	_, current := files["krasis-cli.exe"]
	_, installer := files[installerName]
	if legacy == current {
		return nil, ErrInvalid
	}
	if current {
		if installer {
			return installerNames, nil
		}
		return currentNames, nil
	}
	if installer {
		return nil, ErrInvalid
	}
	return legacyNames, nil
}

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

func (b *Bundle) Metadata() Metadata      { return b.metadata }
func (b *Bundle) HasInputInstaller() bool { return b != nil && b.files[installerName] != nil }
func (b *Bundle) UIExecutable() string {
	if b.files["krasis-cli.exe"] != nil {
		return "krasis.exe"
	}
	return "krasis-control.exe"
}

// MatchDirectory binds staged files back to an authenticated archive's bytes.
// Local checksum metadata alone cannot restore publisher authentication.
func (b *Bundle) MatchDirectory(root interface {
	Open(string) (*os.File, error)
}) error {
	if b == nil {
		return ErrInvalid
	}
	for name, want := range b.files {
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(want)) {
			f.Close()
			return ErrInvalid
		}
		h := sha256.New()
		_, err = io.Copy(h, io.LimitReader(f, int64(len(want))+1))
		closeErr := f.Close()
		expected := sha256.Sum256(want)
		if err != nil || closeErr != nil || !bytes.Equal(h.Sum(nil), expected[:]) {
			return ErrInvalid
		}
	}
	return nil
}

func Read(source io.ReaderAt, size int64) (*Bundle, error) {
	return read(source, size, verifyExecutable)
}

func read(source io.ReaderAt, size int64, executable func(string, []byte, string) error) (*Bundle, error) {
	if source == nil || size <= 0 || size > MaxArchiveBytes {
		return nil, ErrInvalid
	}
	r, err := zip.NewReader(source, size)
	if err != nil || len(r.File) < len(legacyNames) || len(r.File) > len(installerNames)+len(probeNames)+len(uiAccessProbeNames) {
		return nil, ErrInvalid
	}
	files := make(map[string][]byte, len(legacyNames))
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
	base, err := layout(files)
	if err != nil {
		return nil, err
	}
	for _, name := range base {
		if _, ok := files[name]; !ok {
			return nil, ErrInvalid
		}
	}
	if (files[probeNames[0]] == nil) != (files[probeNames[1]] == nil) {
		return nil, ErrInvalid
	}
	uiAccessCount := 0
	for _, name := range uiAccessProbeNames {
		if files[name] != nil {
			uiAccessCount++
		}
	}
	if uiAccessCount != 0 && uiAccessCount != len(uiAccessProbeNames) {
		return nil, ErrInvalid
	}
	selected := selectedNames(base, files[probeNames[0]] != nil, uiAccessCount != 0)
	if len(files) != len(selected) {
		return nil, ErrInvalid
	}
	checksums := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(files["SHA256SUMS.txt"]), "\n"), "\n") {
		parts := strings.SplitN(strings.TrimSuffix(line, "\r"), "  ", 2)
		if len(parts) != 2 || !validHex(parts[0], 64) || fileLimit(parts[1]) == 0 || parts[1] == "SHA256SUMS.txt" || checksums[parts[1]] != "" {
			return nil, ErrInvalid
		}
		checksums[parts[1]] = parts[0]
	}
	if len(checksums) != len(selected)-1 {
		return nil, ErrInvalid
	}
	for _, name := range selected[:len(selected)-1] {
		expected, ok := checksums[name]
		if !ok {
			return nil, ErrInvalid
		}
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
	for _, name := range base[:3] {
		if err := executable(name, files[name], metadata.Revision); err != nil {
			return nil, errors.Join(ErrInvalid, err)
		}
	}
	if files[installerName] != nil {
		if metadata.Generation < InstallerLayoutGeneration {
			return nil, ErrInvalid
		}
		if err := executable(installerName, files[installerName], metadata.Revision); err != nil {
			return nil, errors.Join(ErrInvalid, err)
		}
	}
	return &Bundle{metadata: metadata, files: files}, nil
}

func fileLimit(name string) int64 {
	for index, allowed := range legacyNames {
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
	if name == "krasis-cli.exe" || name == installerName {
		return maxExecutableBytes
	}
	if name == probeNames[0] {
		return 16 << 20
	}
	if name == probeNames[1] {
		return 16 << 10
	}
	for _, allowed := range uiAccessProbeNames {
		if name == allowed {
			if name == uiAccessProbeNames[0] {
				return 16 << 20
			}
			if name == uiAccessProbeNames[1] {
				return 16 << 10
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
	if name == installerName {
		present := false
		for _, tag := range strings.FieldsFunc(settings["-tags"], func(r rune) bool { return r == ',' || r == ' ' }) {
			present = present || tag == "inputinstaller"
		}
		if !present {
			return ErrInvalid
		} // Generic builds deliberately have no payload.
	}
	return nil
}

// Stage creates a fresh directory under a caller-owned, protected root. It never
// overwrites an installed bundle and never uses archive-controlled path names.
func (b *Bundle) Stage(parent *os.Root, directory string) (err error) {
	if b == nil || !validHex(directory, 32) {
		return ErrInvalid
	}
	base, err := layout(b.files)
	if err != nil {
		return err
	}
	uiAccessCount := 0
	for _, name := range uiAccessProbeNames {
		if b.files[name] != nil {
			uiAccessCount++
		}
	}
	if (b.files[probeNames[0]] == nil) != (b.files[probeNames[1]] == nil) || (uiAccessCount != 0 && uiAccessCount != len(uiAccessProbeNames)) {
		return ErrInvalid
	}
	selected := selectedNames(base, b.files[probeNames[0]] != nil, uiAccessCount != 0)
	if len(b.files) != len(selected) {
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
			for _, name := range selected {
				_ = root.Remove(name)
			}
			_ = root.Close()
			_ = parent.Remove(directory)
		}
	}()
	for _, name := range selected {
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
	selected := legacyNames
	legacy, err := root.Open("krasis-control.exe")
	if err == nil {
		legacy.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	current, err := root.Open("krasis-cli.exe")
	if err == nil {
		current.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if (legacy == nil) == (current == nil) {
		return nil, ErrInvalid
	}
	if current != nil {
		selected = currentNames
	}
	installer, err := root.Open(installerName)
	if err == nil {
		installer.Close()
		if current == nil {
			return nil, ErrInvalid
		}
		selected = installerNames
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var present int
	for _, name := range probeNames {
		f, err := root.Open(name)
		if err == nil {
			present++
			f.Close()
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if present == 1 {
		return nil, ErrInvalid
	}
	if present == len(probeNames) {
		selected = selectedNames(selected, true, false)
	}
	uiAccessPresent := 0
	for _, name := range uiAccessProbeNames {
		f, err := root.Open(name)
		if err == nil {
			uiAccessPresent++
			f.Close()
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if uiAccessPresent != 0 && uiAccessPresent != len(uiAccessProbeNames) {
		return nil, ErrInvalid
	}
	if uiAccessPresent == len(uiAccessProbeNames) {
		selected = selectedNames(selected, false, true)
	}
	for _, name := range selected {
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
