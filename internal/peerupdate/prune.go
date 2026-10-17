package peerupdate

import (
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/releaseupdate"
	"github.com/0disoft/krasis/internal/securefs"
)

// Prune runs before the agent exposes management endpoints. Shared UI stages
// younger than a day are retained even when they have not published a receipt.
func (m *Manager) Prune(base string) (int, error) {
	return m.prune(time.Now(), func() ([]string, error) {
		live, err := launchrecord.LiveDirectories()
		if err != nil {
			return nil, err
		}
		recovery, err := launchrecord.RecoveryDirectories(base)
		if err != nil {
			return nil, err
		}
		releases, err := releaseupdate.CachedDirectories(base)
		return append(append(live, recovery...), releases...), err
	})
}

func (m *Manager) prune(now time.Time, protected func() ([]string, error)) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.receiving || m.packing || m.out != nil {
		return 0, ErrBusy
	}
	keep, err := protected()
	if err != nil {
		return 0, err
	}
	// Do not race existing network workers, even when called outside startup.
	if len(m.cancels) != 0 {
		return 0, ErrBusy
	}
	if m.status.State == "READY" || m.status.State == "STAGED" || m.status.State == "APPLYING" {
		keep = append(keep, filepath.Join(m.path, m.status.ID))
	}
	f, err := m.root.Open(".")
	if err != nil {
		return 0, err
	}
	entries, readErr := f.ReadDir(1025)
	closeErr := f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return 0, readErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if len(entries) > 1024 {
		return 0, ErrStorage
	}
	type candidate struct {
		name string
		at   time.Time
	}
	var folders []candidate
	removed := 0
	var failures error
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "archive-") && strings.HasSuffix(name, ".zip") && validID(strings.TrimSuffix(strings.TrimPrefix(name, "archive-"), ".zip"), 32) && entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				failures = errors.Join(failures, err)
				continue
			}
			if now.Sub(info.ModTime()) >= 24*time.Hour {
				if err := m.root.Remove(name); err != nil {
					failures = errors.Join(failures, err)
				} else {
					removed++
				}
			}
			continue
		}
		if !validID(entry.Name(), 32) || !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		folders = append(folders, candidate{entry.Name(), info.ModTime()})
	}
	slices.SortFunc(folders, func(a, b candidate) int {
		if c := b.at.Compare(a.at); c != 0 {
			return c
		}
		return strings.Compare(a.name, b.name)
	})
	for i, folder := range folders {
		path := filepath.Join(m.path, folder.name)
		if i < 2 || now.Sub(folder.at) < 24*time.Hour || slices.ContainsFunc(keep, func(p string) bool { return strings.EqualFold(filepath.Clean(p), path) }) {
			continue
		}
		// Reject junctions/reparse points; deletion itself remains root-relative.
		guard, err := securefs.OpenReadDirectory(path)
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		if err = guard.Close(); err == nil {
			err = m.root.RemoveAll(folder.name)
		}
		if err != nil {
			failures = errors.Join(failures, err)
		} else {
			removed++
		}
	}
	return removed, failures
}
