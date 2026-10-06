package productlog

import (
	"errors"
	"fmt"
	"os"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

const maxCorruptRecoveryArchives = 4

// Replay only a validated product-owned checkpoint. Reads never apply it.
func resumeRecoverySave(dir *securefs.Directory) error {
	pending, err := readRecoveryReceiptNamed(dir, recoveryPendingName)
	if err != nil || pending == nil {
		return err
	}
	_, err = readRecoveryReceiptNamed(dir, "recovery-latest.json")
	if errors.Is(err, errInvalidRecoveryReceipt) {
		if err = archiveCorruptRecovery(dir); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return dir.Replace(recoveryPendingName, "recovery-latest.json")
}

func hasCorruptRecoveryArchive(dir *securefs.Directory) (bool, error) {
	for i := 1; i <= maxCorruptRecoveryArchives; i++ {
		f, err := dir.Open(fmt.Sprintf("recovery-corrupt-%d.json", i))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		err = peercredentials.ValidateSecurityFileHandle(f)
		closeErr := f.Close()
		if err != nil {
			return false, err
		}
		if closeErr != nil {
			return false, closeErr
		}
		return true, nil
	}
	return false, nil
}

// Quarantine malformed content without interpreting or exporting it. Files
// retain their protected ACLs and handle-relative rename protections. Permission
// and I/O failures are not classified as corruption and must still stop saving.
func archiveCorruptRecovery(dir *securefs.Directory) error {
	names := make([]string, maxCorruptRecoveryArchives)
	exists := make([]bool, len(names))
	for i := range names {
		names[i] = fmt.Sprintf("recovery-corrupt-%d.json", i+1)
		f, err := dir.Open(names[i])
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		err = peercredentials.ValidateSecurityFileHandle(f)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		exists[i] = true
	}
	if exists[len(names)-1] {
		if err := dir.Remove(names[len(names)-1]); err != nil {
			return err
		}
	}
	for i := len(names) - 2; i >= 0; i-- {
		if exists[i] {
			if err := dir.PromoteNoReplace(names[i], names[i+1]); err != nil {
				return err
			}
		}
	}
	return dir.PromoteNoReplace("recovery-latest.json", names[0])
}
