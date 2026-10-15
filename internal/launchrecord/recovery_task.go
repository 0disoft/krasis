package launchrecord

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/0disoft/krasis/internal/productidentity"
)

const recoveryTaskRecord = "recovery-task.json"

// UpdateRecoveryTask keeps the scheduler action and its last successful record serialized
// with UI registration. Merely opening another bundle must not lose the task's
// ownership proof before it can be migrated. Failed updates retain that proof.
func UpdateRecoveryTask(base string, enabled bool, apply func(Record, []string) error) error {
	if apply == nil {
		return ErrInvalid
	}
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	unlock, err := lockRegistration(dir)
	if err != nil {
		return err
	}
	defer unlock()
	record, err := Load(base)
	if err != nil {
		return err
	}
	if enabled {
		verified, err := Inspect(base)
		if err != nil || verified != record {
			return ErrInvalid
		}
	}
	previous, err := RecoveryDirectories(base)
	if err != nil {
		return err
	}
	if err := apply(record, previous); err != nil {
		return err
	}
	if !enabled {
		if err := dir.Remove(recoveryTaskRecord); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return replace(dir, recoveryTaskRecord, data)
}
