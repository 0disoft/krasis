package peerupdate

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/releaseupdate"
	"github.com/0disoft/krasis/internal/securefs"
)

const (
	pruneEntryLimit   = 1024
	pruneNewestToKeep = 2
	pruneRetentionAge = 24 * time.Hour
	pruneBundleIDSize = 32
)

type pruneFolder struct {
	name string
	at   time.Time
}

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
		keep := make([]string, 0, len(live)+len(recovery)+len(releases))
		keep = append(keep, live...)
		keep = append(keep, recovery...)
		return append(keep, releases...), err
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
	active := ""
	if m.status.State == "READY" || m.status.State == "STAGED" || m.status.State == "APPLYING" {
		active = filepath.Join(m.path, m.status.ID)
	}
	entries, err := m.readPruneEntries()
	if err != nil {
		return 0, err
	}
	folders, removed, failures, err := m.collectPruneFolders(entries, now)
	if err != nil {
		return 0, err
	}
	n, err := m.pruneFolders(folders, keep, active, now)
	return removed + n, errors.Join(failures, err)
}

func (m *Manager) readPruneEntries() ([]fs.DirEntry, error) {
	f, err := m.root.Open(".")
	if err != nil {
		return nil, err
	}
	entries, readErr := f.ReadDir(pruneEntryLimit + 1)
	closeErr := f.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(entries) > pruneEntryLimit {
		return nil, ErrStorage
	}
	return entries, nil
}

// Archive errors accumulate; an unreadable stage aborts before any stage deletion.
func (m *Manager) collectPruneFolders(entries []fs.DirEntry, now time.Time) (folders []pruneFolder, removed int, failures, fatal error) {
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "archive-") && strings.HasSuffix(name, ".zip") && validID(strings.TrimSuffix(strings.TrimPrefix(name, "archive-"), ".zip"), pruneBundleIDSize) && entry.Type().IsRegular() {
			n, err := m.pruneArchive(entry, now)
			removed += n
			failures = errors.Join(failures, err)
			continue
		}
		if !validID(name, pruneBundleIDSize) || !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, 0, nil, err
		}
		folders = append(folders, pruneFolder{name, info.ModTime()})
	}
	// Sort only the locally collected metadata, never a caller-owned slice.
	slices.SortFunc(folders, func(a, b pruneFolder) int {
		if c := b.at.Compare(a.at); c != 0 {
			return c
		}
		return strings.Compare(a.name, b.name)
	})
	return folders, removed, failures, nil
}

func (m *Manager) pruneArchive(entry fs.DirEntry, now time.Time) (int, error) {
	info, err := entry.Info()
	if err != nil {
		return 0, err
	}
	if now.Sub(info.ModTime()) < pruneRetentionAge {
		return 0, nil
	}
	if err := m.root.Remove(entry.Name()); err != nil {
		return 0, err
	}
	return 1, nil
}

func (m *Manager) pruneFolders(folders []pruneFolder, keep []string, active string, now time.Time) (int, error) {
	removed := 0
	var failures error
	for _, name := range planPruneFolders(folders, keep, active, m.path, now) {
		// Reject junctions/reparse points; deletion itself remains root-relative.
		guard, err := securefs.OpenReadDirectory(filepath.Join(m.path, name))
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		if err = guard.Close(); err == nil {
			err = m.root.RemoveAll(name)
		}
		if err != nil {
			failures = errors.Join(failures, err)
		} else {
			removed++
		}
	}
	return removed, failures
}

// planPruneFolders receives newest-first metadata with validated lowercase hex
// folder IDs from collectPruneFolders and does no I/O or mutation.
func planPruneFolders(folders []pruneFolder, keep []string, active, base string, now time.Time) []string {
	protected := make(map[string]struct{}, len(keep)+1)
	protect := func(path string) {
		name := strings.ToLower(filepath.Base(path))
		// Collected IDs contain only lowercase hex. Compare the complete path
		// once per protection hint so EqualFold's Unicode rules and the parent
		// directory still decide whether that ID belongs to this stage root.
		if strings.EqualFold(path, filepath.Join(base, name)) {
			protected[name] = struct{}{}
		}
	}
	for _, path := range keep {
		protect(filepath.Clean(path))
	}
	if active != "" {
		protect(active)
	}
	var names []string
	for i, folder := range folders {
		if i < pruneNewestToKeep || now.Sub(folder.at) < pruneRetentionAge {
			continue
		}
		if _, found := protected[folder.name]; found {
			continue
		}
		names = append(names, folder.name)
	}
	return names
}
