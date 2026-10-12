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

const inputHealthName = "input-health-latest.json"

// Local UI observation; not permission, live service status or install outcome.
type InputHealthSummary struct {
	Time       string `json:"time"`
	State      string `json:"state"`
	Stage      string `json:"stage"`
	Win32Error uint32 `json:"win32Error"`
	Win32Known bool   `json:"win32Known"`
}

func (v InputHealthSummary) valid() bool {
	if _, err := time.Parse(time.RFC3339Nano, v.Time); err != nil {
		return false
	}
	switch v.State {
	case "READY", "UNKNOWN", "PENDING", "NOT_INSTALLED", "UPDATE_REQUIRED", "SERVICE_STOPPED":
	default:
		return false
	}
	switch v.Stage {
	case "complete", "transaction", "bundle_read", "installation", "installation_close", "agent_launch", "service_manager", "service_query", "service_missing", "service_state", "service_record", "service_identity":
	default:
		return false
	}
	return v.Win32Known && v.Win32Error != 0 || !v.Win32Known && v.Win32Error == 0
}
func SaveInputHealth(base string, value InputHealthSummary) error {
	if !value.valid() {
		return errors.New("invalid input health observation")
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	f, name, err := dir.CreateTemporary(".input-health-", 0600)
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
	return dir.Replace(name, inputHealthName)
}
func readInputHealth(dir *securefs.Directory) (*InputHealthSummary, error) {
	f, err := dir.Open(inputHealthName)
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
	data, err := io.ReadAll(io.LimitReader(f, 2049))
	if err != nil || len(data) > 2048 {
		return nil, errors.New("invalid input health observation")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var v InputHealthSummary
	if decoder.Decode(&v) != nil || !v.valid() || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid input health observation")
	}
	return &v, nil
}
