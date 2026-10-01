package productlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/0disoft/krasis/internal/installationdiag"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

const installationReceiptName = "installation-latest.json"

// SaveInstallation stores only a validated fixed-code/numeric receipt.
func SaveInstallation(base string, summary installationdiag.Summary) error {
	if !summary.Valid() {
		return errors.New("invalid installation receipt")
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	file, name, err := dir.CreateTemporary(".installation-", 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(name)
	defer file.Close()
	if err = peercredentials.ProtectSecurityFileHandle(file); err != nil {
		return err
	}
	if err = json.NewEncoder(file).Encode(summary); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return dir.Replace(name, installationReceiptName)
}

func readInstallationReceipt(dir *securefs.Directory) (*installationdiag.Summary, error) {
	file, err := dir.Open(installationReceiptName)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err = peercredentials.ValidateSecurityFileHandle(file); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, 2049))
	if err != nil || len(data) > 2048 {
		return nil, errors.New("invalid installation receipt")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var summary installationdiag.Summary
	if decoder.Decode(&summary) != nil || !summary.Valid() || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid installation receipt")
	}
	return &summary, nil
}
