package productlog

import (
	"errors"
	"os"
	"strings"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

func requestPruneName(id string) string { return "request-prune-" + id + ".pending" }

// The protected empty marker records a request ID, never caller-supplied paths.
// Keep it until every matching snapshot is removed, including across restarts.
func markRequestPrune(dir *securefs.Directory, id string) error {
	f, err := dir.CreateExclusive(requestPruneName(id), 0600)
	if errors.Is(err, os.ErrExist) {
		return validateRequestPrune(dir, id)
	}
	if err != nil {
		return err
	}
	err = peercredentials.ProtectSecurityFileHandle(f)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func validateRequestPrune(dir *securefs.Directory, id string) error {
	f, err := dir.Open(requestPruneName(id))
	if err != nil {
		return err
	}
	defer f.Close()
	if err = peercredentials.ValidateSecurityFileHandle(f); err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() != 0 {
		return errors.New("invalid diagnostic pruning checkpoint")
	}
	return nil
}

// Callers hold requestSnapshotGate while collecting and consuming names. Each
// group keeps enumeration order; all file validation remains handle-relative.
func finishRequestPrune(dir *securefs.Directory, id string, names []string, remove func(string) error) error {
	if err := validateRequestPrune(dir, id); err != nil {
		return err
	}
	for _, name := range names {
		if snapshotRequestID(name) != id {
			continue
		}
		f, err := dir.Open(name)
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
		if err = remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return dir.Remove(requestPruneName(id))
}

func resumeRequestPruning(dir *securefs.Directory) error {
	names, err := dir.ReadNames()
	if err != nil {
		return err
	}
	var pending []string
	for _, name := range names {
		if !strings.HasPrefix(name, "request-prune-") || !strings.HasSuffix(name, ".pending") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "request-prune-"), ".pending")
		if !validRequestID(id) {
			continue
		}
		pending = append(pending, id)
	}
	if len(pending) == 0 {
		return nil
	}
	byRequest := make(map[string][]string)
	for _, name := range names {
		if id := snapshotRequestID(name); id != "" {
			byRequest[id] = append(byRequest[id], name)
		}
	}
	for _, id := range pending {
		if err = finishRequestPrune(dir, id, byRequest[id], dir.Remove); err != nil {
			return err
		}
	}
	return nil
}
