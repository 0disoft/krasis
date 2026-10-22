package peerupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTimedOutCloseStillCleansUpAfterLastReader(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	archive := queuedFileOffer(t, m, peer)
	lease := m.offered(peer)
	defer lease.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Deterministically model expiration while a reader is still active.
	if err := m.closeContext(ctx); !errors.Is(err, ErrBusy) {
		t.Fatal("close did not respect the caller's deadline", err)
	}
	done := m.closeDone
	if err := m.closeContext(ctx); !errors.Is(err, ErrBusy) || m.closeDone != done {
		t.Fatal("another waiter started another shutdown", err)
	}
	if m.offered(peer) != nil {
		t.Fatal("shutdown admitted a new reader")
	}
	if _, err := io.ReadAll(lease.reader); err != nil {
		t.Fatal("shutdown closed a file still owned by a reader", err)
	}
	lease.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup required a second Close call")
	}
	if m.closeErr != nil {
		t.Fatal(m.closeErr)
	}
	if _, err := os.Stat(filepath.Join(m.path, archive.name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("shutdown retained the queued archive", err)
	}
	if _, err := m.root.Stat("."); !errors.Is(err, os.ErrClosed) {
		t.Fatal("shutdown retained the root handle", err)
	}
	if _, err := archive.file.ReadAt(make([]byte, 1), 0); !errors.Is(err, os.ErrClosed) {
		t.Fatal("shutdown retained the archive handle", err)
	}
	if err := m.Close(); err != nil {
		t.Fatal("repeated close repeated cleanup", err)
	}
}

func TestConcurrentCloseWaitersJoinOneShutdown(t *testing.T) {
	m := testManager(t)
	lease := queuedFileOffer(t, m, strings.Repeat("a", 64))
	results := make(chan error, 4)
	for range cap(results) {
		go func() { results <- m.Close() }()
	}
	for range cap(results) {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal("concurrent close repeated cleanup", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("close waiter did not complete")
		}
	}
	if _, err := lease.file.ReadAt(make([]byte, 1), 0); !errors.Is(err, os.ErrClosed) {
		t.Fatal("concurrent shutdown retained the archive handle", err)
	}
}
