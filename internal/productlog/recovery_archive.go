package productlog

import (
	"errors"
	"fmt"
	"os"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

const maxCorruptRecoveryArchives = 4

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
