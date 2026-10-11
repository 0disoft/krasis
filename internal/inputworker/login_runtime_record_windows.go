//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/logindiag"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"time"
)

func saveLoginRuntime(v logindiag.Runtime) error {
	if !v.Valid() {
		return ErrAuthority
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return err
	}
	root = filepath.Join(root, "Krasis")
	pinned, err := openProtected(root, true)
	if err != nil {
		return err
	}
	defer pinned.Close()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pending := filepath.Join(root, "LOGIN-RUNTIME.pending")
	if err := writeInstallFile(ctx, pending, data); err != nil {
		return err
	}
	destination := filepath.Join(root, "LOGIN-RUNTIME.json")
	if !missingInstallPath(destination) {
		previous, err := openProtected(destination, false)
		if err != nil {
			return err
		}
		if err := previous.Close(); err != nil {
			return err
		}
	}
	return moveInstallPath(pending, destination, true)
}

// ReadLoginRuntime reads only the fixed machine-owned decision receipt. A
// historical STARTED entry does not prove a live worker or input connection.
func ReadLoginRuntime() (*logindiag.Runtime, error) {
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return nil, err
	}
	root = filepath.Join(root, "Krasis")
	pinned, err := openProtected(root, true)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer pinned.Close()
	file, err := openProtected(filepath.Join(root, "LOGIN-RUNTIME.json"), false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 2049))
	if err != nil || len(data) > 2048 {
		return nil, ErrAuthority
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var v logindiag.Runtime
	if d.Decode(&v) != nil || !v.Valid() || d.Decode(new(any)) != io.EOF {
		return nil, ErrAuthority
	}
	return &v, nil
}
