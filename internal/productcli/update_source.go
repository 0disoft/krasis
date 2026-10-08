package productcli

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"

	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

// Only the fixed recovery CLI resolves a recorded bundle. Portable invocations
// keep their adjacent bundle, including failures; never search arbitrary paths.
func updateBundleExecutable(base, executable string) (string, error) {
	fixed, err := launchrecord.Directory(base)
	if err != nil || !strings.EqualFold(filepath.Clean(executable), filepath.Join(fixed, "krasis-cli.exe")) {
		return executable, nil
	}
	record, err := launchrecord.Inspect(base)
	if err != nil {
		return "", err
	}
	directory := filepath.Dir(record.Executable)
	root, err := securefs.OpenReadDirectory(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	manifest, err := root.Open("BUILD.json")
	if err != nil {
		return "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(manifest, 8193))
	closeErr := manifest.Close()
	var metadata updatebundle.Metadata
	if readErr != nil || closeErr != nil || len(data) > 8192 || json.Unmarshal(data, &metadata) != nil || metadata.Generation != record.Generation || metadata.Revision != record.Revision {
		return "", launchrecord.ErrInvalid
	}
	candidate := filepath.Join(directory, "krasis-cli.exe")
	// Registration copies the CLI. A changed or stale copy must not silently
	// send a different version from the one the operator actually invoked.
	currentHash, err := updateCLIHash(executable)
	if err != nil {
		return "", err
	}
	candidateHash, err := updateCLIHash(candidate)
	if err != nil || currentHash != candidateHash {
		return "", launchrecord.ErrInvalid
	}
	return candidate, nil
}

func updateCLIHash(path string) ([32]byte, error) {
	var digest [32]byte
	dir, err := securefs.OpenReadDirectory(filepath.Dir(path))
	if err != nil {
		return digest, err
	}
	defer dir.Close()
	f, err := dir.Open(filepath.Base(path))
	if err != nil {
		return digest, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<20 {
		return digest, launchrecord.ErrInvalid
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, (64<<20)+1))
	if err != nil {
		return digest, err
	}
	if n != info.Size() {
		return digest, launchrecord.ErrInvalid
	}
	copy(digest[:], h.Sum(nil))
	return digest, nil
}
