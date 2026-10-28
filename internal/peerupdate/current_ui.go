package peerupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

// RelaunchCurrentUI uses only locally staged, fully verified bytes matching this
// agent. No remote path/arguments or newer/older generation is accepted. The UI
// promotes its launch record only after its own startup succeeds.
func (m *Manager) RelaunchCurrentUI(ctx context.Context, start func(string) error) error {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed || start == nil {
		return ErrDenied
	}
	return relaunchCurrentUI(ctx, m.path, m.generation, m.revision, verifyCurrentUIBundle, start)
}

func verifyCurrentUIBundle(dir *securefs.Directory) (updatebundle.Metadata, error) {
	var archive bytes.Buffer
	if err := updatebundle.PackTo(dir, &archive); err != nil {
		return updatebundle.Metadata{}, err
	}
	bundle, err := updatebundle.Read(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		return updatebundle.Metadata{}, err
	}
	return bundle.Metadata(), nil
}

func relaunchCurrentUI(ctx context.Context, root string, generation int, revision string,
	verify func(*securefs.Directory) (updatebundle.Metadata, error), start func(string) error) error {
	if ctx == nil || ctx.Err() != nil || generation < 21 || !validID(revision, 40) {
		return ErrDenied
	}
	directory, err := securefs.OpenReadDirectory(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	rootPin, err := pinCurrentUIPath(root, true)
	if err != nil {
		return err
	}
	defer rootPin.Close()
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootFS.Close()
	folders, err := rootFS.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := folders.ReadDir(1025)
	closeErr := folders.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if len(entries) > 1024 {
		return ErrStorage
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() || !validID(entry.Name(), 32) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		dir, err := securefs.OpenReadDirectory(path)
		if err != nil {
			continue
		} // Never follow a junction or other unknown stage.
		pin, err := pinCurrentUIPath(path, true)
		if err != nil {
			dir.Close()
			continue
		}
		matched, err := launchMatchingUIStage(ctx, dir, path, generation, revision, verify, start)
		closeErr := errors.Join(pin.Close(), dir.Close())
		if matched {
			return errors.Join(err, closeErr)
		}
	}
	return os.ErrNotExist
}

func launchMatchingUIStage(ctx context.Context, dir *securefs.Directory, path string, generation int, revision string,
	verify func(*securefs.Directory) (updatebundle.Metadata, error), start func(string) error) (bool, error) {
	manifest, err := dir.Open("BUILD.json")
	if err != nil {
		return false, nil
	}
	defer manifest.Close()
	var hint updatebundle.Metadata
	decoder := json.NewDecoder(io.LimitReader(manifest, 4097))
	if decoder.Decode(&hint) != nil || decoder.Decode(new(any)) != io.EOF || hint.Generation != generation || hint.Revision != revision {
		return false, nil
	}
	manifestPin, err := pinCurrentUIPath(filepath.Join(path, "BUILD.json"), false)
	if err != nil {
		return true, err
	}
	defer manifestPin.Close()
	image, err := pinCurrentUIPath(filepath.Join(path, "krasis.exe"), false)
	if err != nil {
		return true, err
	}
	defer image.Close() // Pin the executable across verification and process creation.
	metadata, err := verify(dir)
	if err != nil {
		return true, err
	}
	if metadata.Generation != generation || metadata.Revision != revision {
		return true, ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	return true, start(filepath.Join(path, "krasis.exe"))
}
