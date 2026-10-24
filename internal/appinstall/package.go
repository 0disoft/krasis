// Package appinstall owns the per-user application installation, separate from
// the elevated, machine-wide protected-input component.
package appinstall

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/updatebundle"
	"github.com/0disoft/krasis/internal/ziplimit"
	"io"
	"slices"
)

var ErrInvalid = errors.New("invalid application installation")
var ErrBusy = errors.New("close Krasis before uninstalling")
var ErrOlder = errors.New("use the latest application setup")

const Schema = "krasis.app-install/v1"

var files = []string{"BUILD.json", "SHA256SUMS.txt", "START-HERE.md", "THIRD-PARTY-NOTICES.txt", "krasis.exe", "krasis-agent.exe", "krasis-cli.exe", "krasis-install.exe"}

type Receipt struct {
	Schema     string   `json:"schema"`
	Generation int      `json:"generation"`
	Revision   string   `json:"revision"`
	Folders    []string `json:"folders"`
}
type Package struct {
	Bundle *updatebundle.Bundle
	ID     string
}

func Read(data []byte) (Package, error) {
	if int64(len(data)) > updatebundle.MaxArchiveBytes {
		return Package{}, ErrInvalid
	}
	z, err := ziplimit.NewReader(bytes.NewReader(data), int64(len(data)), len(files))
	if err != nil || len(z.File) != len(files) {
		return Package{}, ErrInvalid
	}
	for _, f := range z.File {
		if !slices.Contains(files, f.Name) {
			return Package{}, ErrInvalid
		}
	}
	b, err := updatebundle.Read(bytes.NewReader(data), int64(len(data)))
	if err != nil || b.Metadata().Generation < 234 || !b.HasInputInstaller() {
		return Package{}, ErrInvalid
	}
	h := sha256.Sum256(data)
	return Package{b, hex.EncodeToString(h[:16])}, nil
}
func DecodeReceipt(data []byte) (Receipt, error) {
	var r Receipt
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if len(data) > 8192 || d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || r.Schema != Schema || r.Generation < 234 || r.Generation > 1000000 || len(r.Revision) != 40 || len(r.Folders) < 1 || len(r.Folders) > 64 {
		return Receipt{}, ErrInvalid
	}
	if _, err := hex.DecodeString(r.Revision); err != nil {
		return Receipt{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, name := range r.Folders {
		if len(name) != 32 || seen[name] {
			return Receipt{}, ErrInvalid
		}
		decoded, err := hex.DecodeString(name)
		if err != nil || hex.EncodeToString(decoded) != name {
			return Receipt{}, ErrInvalid
		}
		seen[name] = true
	}
	return r, nil
}
