package productlog

import (
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

func validRequestID(id string) bool {
	_, err := hex.DecodeString(id)
	return err == nil && len(id) == 32 && id == strings.ToLower(id)
}

func readSupportSnapshot(dir *securefs.Directory, name string) ([]byte, error) {
	f, err := dir.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = peercredentials.ValidateSecurityFileHandle(f); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxSupportBytes+1))
	if err != nil {
		return nil, err
	}
	return SanitizeSupport(data)
}

// ExportForRequest pins a sanitized local snapshot to one collection ID.
// Repeated requests reuse the original file; routine exports never replace it.
func ExportForRequest(base, id string) (string, error) {
	if !validRequestID(id) {
		return "", errors.New("invalid diagnostic request")
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	name := "support-request-" + id + "-local.json"
	if _, err = readSupportSnapshot(dir, name); errors.Is(err, os.ErrNotExist) {
		err = exportToNamed(dir, name, true)
		if errors.Is(err, os.ErrExist) {
			_, err = readSupportSnapshot(dir, name)
		}
	}
	if err != nil {
		return "", err
	}
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return "", err
	}
	return filepath.Join(paths.Diagnostics, name), nil
}
