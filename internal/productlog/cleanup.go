package productlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

type CleanupSummary struct {
	Time    string `json:"time"`
	Code    string `json:"code"`
	Removed int    `json:"removed"`
}

func (v CleanupSummary) valid() bool {
	_, err := time.Parse(time.RFC3339Nano, v.Time)
	return err == nil && v.Removed >= 0 && v.Removed <= 1024 && (v.Code == "completed" || v.Code == "failed")
}

func SaveCleanup(base string, value CleanupSummary) error {
	if !value.valid() {
		return errors.New("invalid cleanup receipt")
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	f, name, err := dir.CreateTemporary(".cleanup-", 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(name)
	defer f.Close()
	if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
		return err
	}
	if err = json.NewEncoder(f).Encode(value); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return dir.Replace(name, "cleanup-latest.json")
}

// CleanupReceipt is read-only, including when diagnostics have never existed.
func CleanupReceipt(base string) (*CleanupSummary, error) {
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return nil, err
	}
	if _, err = os.Lstat(paths.Diagnostics); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if _, err = peercredentials.ValidateSecurityDirectory(paths.Diagnostics); err != nil {
		return nil, err
	}
	dir, err := securefs.OpenReadDirectory(paths.Diagnostics)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return readCleanupReceipt(dir)
}

func readCleanupReceipt(dir *securefs.Directory) (*CleanupSummary, error) {
	f, err := dir.Open("cleanup-latest.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = peercredentials.ValidateSecurityFileHandle(f); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil || len(data) > 1024 {
		return nil, errors.New("invalid cleanup receipt")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var value CleanupSummary
	if d.Decode(&value) != nil || !value.valid() || d.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid cleanup receipt")
	}
	return &value, nil
}
