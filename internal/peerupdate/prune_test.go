package peerupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
