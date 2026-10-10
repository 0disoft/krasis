package productlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

const loginConfigurationReceiptName = "login-configuration-latest.json"

// SaveLoginConfiguration stores only a validated fixed-code/numeric receipt.
func SaveLoginConfiguration(base string, summary logindiag.Summary) error {
	if !summary.Valid() {
		return errors.New("invalid login configuration receipt")
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	file, name, err := dir.CreateTemporary(".login-configuration-", 0600)
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
	return dir.Replace(name, loginConfigurationReceiptName)
}

func readLoginConfigurationReceipt(dir *securefs.Directory) (*logindiag.Summary, error) {
	file, err := dir.Open(loginConfigurationReceiptName)
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
		return nil, errors.New("invalid login configuration receipt")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var summary logindiag.Summary
	if decoder.Decode(&summary) != nil || !summary.Valid() || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid login configuration receipt")
	}
	return &summary, nil
}
