package peerupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPruneDoesNotModifyProtectedBackingArray(t *testing.T) {
	for _, state := range []string{"READY", "STAGED", "APPLYING"} {
		t.Run(state, func(t *testing.T) {
			m := testManager(t)
			m.status = Status{State: state, ID: strings.Repeat("a", 32)}
			backing := []string{filepath.Join(m.path, "protected"), "caller-owned-sentinel"}
			before := slices.Clone(backing)
			n, err := m.prune(time.Now(), func() ([]string, error) { return backing[:1], nil })
			if err != nil || n != 0 {
				t.Fatalf("removed=%d err=%v", n, err)
			}
			if !slices.Equal(backing, before) {
				t.Fatalf("protected callback's backing array changed: %v", backing)
			}
		})
	}
}

func TestPlanPruneFoldersRetentionAndProtection(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	base := filepath.Join("stages", "updates")
	for _, tc := range []struct {
		name   string
		age    time.Duration
		keep   []string
		active string
		remove bool
	}{
		{name: "before cutoff", age: 24*time.Hour - time.Nanosecond},
		{name: "at cutoff", age: 24 * time.Hour, remove: true},
		{name: "after cutoff", age: 24*time.Hour + time.Nanosecond, remove: true},
		{name: "future timestamp", age: -time.Hour},
		{name: "protected normalized case", age: 48 * time.Hour, keep: []string{
			strings.ToUpper(base + string(filepath.Separator) + "unused" + string(filepath.Separator) + ".." + string(filepath.Separator) + "old"),
		}},
		{name: "active stage", age: 48 * time.Hour, active: filepath.Join(base, "old")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			folders := []pruneFolder{{"newest", now}, {"second", now.Add(-time.Hour)}, {"old", now.Add(-tc.age)}}
			before, keepBefore := slices.Clone(folders), slices.Clone(tc.keep)
			got := planPruneFolders(folders, tc.keep, tc.active, base, now)
			var want []string
			if tc.remove {
				want = []string{"old"}
			}
			if !slices.Equal(got, want) {
				t.Fatalf("plan=%v want=%v", got, want)
			}
			if !slices.Equal(folders, before) || !slices.Equal(tc.keep, keepBefore) {
				t.Fatal("plan mutated its input")
			}
		})
	}
	folders := []pruneFolder{{"newest", now.Add(-48 * time.Hour)}, {"second", now.Add(-72 * time.Hour)}, {"old", now.Add(-96 * time.Hour)}}
	if got := planPruneFolders(folders, nil, "", base, now); !slices.Equal(got, []string{"old"}) {
		t.Fatalf("newest two stages must survive even when old: %v", got)
	}
}

func TestPrunePreservesGuardOrder(t *testing.T) {
	m := testManager(t)
	ownerErr := errors.New("unknown owner")
	calls := 0
	protected := func() ([]string, error) { calls++; return nil, ownerErr }
	m.receiving = true
	if _, err := m.prune(time.Now(), protected); !errors.Is(err, ErrBusy) || calls != 0 {
		t.Fatalf("busy check must precede ownership lookup: calls=%d err=%v", calls, err)
	}
	m.receiving = false
	m.cancels = map[uint64]context.CancelFunc{1: func() {}}
	if _, err := m.prune(time.Now(), protected); err != ownerErr || calls != 1 {
		t.Fatalf("ownership error must precede worker check: calls=%d err=%v", calls, err)
	}
	m.cancels = nil
}

func TestPruneKeepsRecentActiveReadyAndUnknownNames(t *testing.T) {
	m := testManager(t)
	now := time.Now()
	var ids []string
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("%032x", i)
		ids = append(ids, id)
		path := filepath.Join(m.path, id)
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "krasis.exe"), []byte("MZ"), 0600); err != nil {
			t.Fatal(err)
		}
		at := now.Add(-time.Duration(i+1) * 24 * time.Hour)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(m.path, "not-a-bundle"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.path, ids[7])
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
	m.status = Status{State: "APPLYING", ID: ids[5]}
	keep := func() ([]string, error) {
		return []string{filepath.Join(m.path, ids[3]), filepath.Join(m.path, ids[4])}, nil
	}
	n, err := m.prune(now, keep)
	if err != nil || n != 3 {
		t.Fatalf("removed=%d err=%v", n, err)
	}
	for _, id := range []string{ids[7], ids[0], ids[3], ids[4], ids[5], "not-a-bundle"} {
		if _, err := os.Stat(filepath.Join(m.path, id)); err != nil {
			t.Fatalf("protected folder %s: %v", id, err)
		}
	}
	// A failed process/receipt inspection must prevent any deletion.
	n, err = m.prune(now, func() ([]string, error) { return nil, errors.New("unknown owner") })
	if err == nil || n != 0 {
		t.Fatal("unknown owner allowed cleanup")
	}
	m.cancels = map[uint64]context.CancelFunc{1: func() {}}
	if _, err = m.prune(now, keep); !errors.Is(err, ErrBusy) {
		t.Fatal("active network worker allowed cleanup")
	}
	m.cancels = nil
}
