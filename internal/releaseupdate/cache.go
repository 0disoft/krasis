package releaseupdate

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

type readyReceipt struct {
	Envelope []byte            `json:"envelope"`
	Folders  []string          `json:"folders"`
	History  map[string][]byte `json:"history"`
}

const maxReceiptBytes = 2 << 20 // 64 bounded signed envelopes, base64 encoded.

func readCache(dir *securefs.Directory, name string, limit int64) ([]byte, error) {
	f, err := dir.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = peercredentials.ValidateSecurityFileHandle(f); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, ErrInvalid
	}
	return data, nil
}

func writeCache(dir *securefs.Directory, name string, data []byte) error {
	f, temp, err := dir.CreateTemporary(".release-", 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(temp)
	if err = peercredentials.ProtectSecurityFileHandle(f); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return dir.Replace(temp, name)
}

func loadReady(dir *securefs.Directory) (readyReceipt, error) {
	data, err := readCache(dir, "release-ready.json", maxReceiptBytes)
	if err != nil {
		return readyReceipt{}, err
	}
	var r readyReceipt
	if decode(data, &r) != nil || len(r.Folders) == 0 || len(r.Folders) > 64 {
		return readyReceipt{}, ErrInvalid
	}
	for _, id := range r.Folders {
		if !lowerHex(id, 32) {
			return readyReceipt{}, ErrInvalid
		}
	}
	return r, nil
}

// SaveReady persists the publisher's signed envelope AND its original archive.
// A torn two-file write is rejected on restore. Repeated checks reuse one stage.
func SaveReady(base string, envelope, archive []byte, key string) (string, Manifest, error) {
	m, err := Verify(envelope, key)
	if err != nil {
		return "", m, err
	}
	b, err := verifyArchive(archive, m)
	if err != nil {
		return "", m, err
	}
	id := m.SHA256[:32]
	directory, err := stage(base, b, id)
	if errors.Is(err, os.ErrExist) {
		paths, _ := productidentity.ResolveWindowsPaths(base)
		directory = filepath.Join(paths.Root, "updates", id)
		guard, e := securefs.OpenReadDirectory(directory)
		if e != nil {
			return "", m, e
		}
		err = b.MatchDirectory(guard)
		guard.Close()
	}
	if err != nil {
		return "", m, err
	}
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		return "", m, err
	}
	defer dir.Close()
	r := readyReceipt{Envelope: envelope, Folders: []string{id}, History: map[string][]byte{id: envelope}}
	if old, e := loadReady(dir); e == nil {
		for _, entry := range old.Folders {
			if !slices.Contains(r.Folders, entry) {
				r.Folders = append(r.Folders, entry)
				r.History[entry] = old.History[entry]
			}
		}
	}
	if len(r.Folders) > 64 {
		return "", m, ErrInvalid
	}
	if err = writeCache(dir, "release-ready.zip", archive); err != nil {
		return "", m, err
	}
	data, _ := json.Marshal(r)
	if err = writeCache(dir, "release-ready.json", data); err != nil {
		return "", m, err
	}
	return directory, m, nil
}

func RestoreReady(base, key string, current int) (string, Manifest, error) {
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		return "", Manifest{}, err
	}
	defer dir.Close()
	r, err := loadReady(dir)
	if err != nil {
		return "", Manifest{}, err
	}
	m, err := Verify(r.Envelope, key)
	if err != nil {
		return "", m, err
	}
	if m.Generation <= current {
		return "", m, os.ErrNotExist
	}
	if r.Folders[0] != m.SHA256[:32] {
		return "", m, ErrInvalid
	}
	archive, err := readCache(dir, "release-ready.zip", m.Size)
	if err != nil {
		return "", m, err
	}
	b, err := verifyArchive(archive, m)
	if err != nil {
		return "", m, err
	}
	paths, _ := productidentity.ResolveWindowsPaths(base)
	directory := filepath.Join(paths.Root, "updates", r.Folders[0])
	guard, err := securefs.OpenReadDirectory(directory)
	if err != nil {
		return "", m, err
	}
	defer guard.Close()
	if err = b.MatchDirectory(guard); err != nil {
		return "", m, err
	}
	return directory, m, nil
}

// PruneReady removes only recorded public-release stages. Keep the two newest,
// the running UI and the recovery record. Peer updates are never enumerated.
func PruneReady(base, key string) error {
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	r, err := loadReady(dir)
	if err != nil {
		return err
	}
	paths, _ := productidentity.ResolveWindowsPaths(base)
	parent := filepath.Join(paths.Root, "updates")
	if _, err = peercredentials.ValidateSecurityDirectory(parent); err != nil {
		return err
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return err
	}
	defer root.Close()
	live, err := launchrecord.LiveDirectories()
	if err != nil {
		return err
	}
	latest, _ := launchrecord.Load(base)
	kept := []string{}
	for i, id := range r.Folders {
		m, e := Verify(r.History[id], key)
		if e != nil || m.SHA256[:32] != id {
			kept = append(kept, id)
			continue
		}
		folder := filepath.Join(parent, id)
		active := slices.ContainsFunc(live, func(path string) bool { return strings.EqualFold(path, folder) })
		if i < 2 || active || strings.EqualFold(filepath.Dir(latest.Executable), folder) {
			kept = append(kept, id)
			continue
		}
		guard, e := securefs.OpenReadDirectory(folder)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			kept = append(kept, id)
			continue
		}
		guard.Close()
		if e = root.RemoveAll(id); e != nil {
			kept = append(kept, id)
		}
	}
	r.Folders = kept
	for id := range r.History {
		if !slices.Contains(kept, id) {
			delete(r.History, id)
		}
	}
	data, _ := json.Marshal(r)
	return writeCache(dir, "release-ready.json", data)
}
