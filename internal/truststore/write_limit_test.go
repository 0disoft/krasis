package truststore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/transport"
)

// Build a valid document with an exact persisted size, including its newline.
func documentAtSize(t *testing.T, size int) Document {
	t.Helper()
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	doc := Document{SchemaVersion: SchemaVersion, Revision: 1, UpdatedAt: now}
	peers := make([]PeerRecord, 8192)
	for i := range peers {
		peers[i] = PeerRecord{Fingerprint: fmt.Sprintf("%064x", i+1),
			Role: transport.AuthenticatedPeerConnector, Alias: "p", CreatedAt: now}
	}
	encodedSize := func() int {
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return len(data) + 1
	}
	low, high := 1, len(peers)
	for low < high {
		mid := (low + high + 1) / 2
		doc.Peers = peers[:mid]
		if encodedSize() <= size {
			low = mid
		} else {
			high = mid - 1
		}
	}
	doc.Peers = peers[:low]
	remaining := size - encodedSize()
	for i := len(doc.Peers) - 1; remaining > 0 && i > 0; i-- {
		grow := min(63, remaining)
		doc.Peers[i].Alias = strings.Repeat("p", 1+grow)
		remaining -= grow
	}
	if remaining != 0 || encodedSize() != size {
		t.Fatal("fixture did not reach requested size")
	}
	if err := doc.Validate(); err != nil {
		t.Fatal(err)
	}
	return doc
}

func seedSizedDocument(t *testing.T, store *Store, doc Document) {
	t.Helper()
	if err := store.mutate(true, doc.UpdatedAt, func(current *Document) error {
		*current = doc
		current.Revision-- // mutate increments the published revision.
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOversizedTrustUpdatePreservesReadableStore(t *testing.T) {
	store := mustStore(t)
	doc := documentAtSize(t, maxStoreBytes-16)
	seedSizedDocument(t, store, doc)
	path := filepath.Join(store.directory, storeFilename)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, updateErr := store.Update(doc.Peers[0].Fingerprint, strings.Repeat("p", 64), false, doc.UpdatedAt)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := store.List()
	if updateErr == nil || !bytes.Equal(before, after) || readErr != nil {
		t.Fatalf("oversized update damaged trust: update=%v, bytes=%d -> %d, read=%v", updateErr, len(before), len(after), readErr)
	}
}

func TestOversizedTrustMutationSkipsPreparation(t *testing.T) {
	store := mustStore(t)
	doc := documentAtSize(t, maxStoreBytes-16)
	seedSizedDocument(t, store, doc)
	prepared := false
	err := store.mutatePrepared(false, doc.UpdatedAt, func(current *Document) error {
		current.Peers[0].Alias = strings.Repeat("p", 64)
		return nil
	}, func(_ *securefs.Directory) error { prepared = true; return nil })
	if err == nil || prepared {
		t.Fatalf("oversized candidate reached certificate preparation: err=%v, prepared=%v", err, prepared)
	}
}

func TestTrustWriteSizeBoundaryIncludesNewline(t *testing.T) {
	for _, size := range []int{maxStoreBytes - 1, maxStoreBytes, maxStoreBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			store := mustStore(t)
			doc := documentAtSize(t, size)
			err := store.mutate(true, doc.UpdatedAt, func(current *Document) error {
				*current = doc
				current.Revision--
				return nil
			})
			path := filepath.Join(store.directory, storeFilename)
			if size > maxStoreBytes {
				if err == nil {
					t.Fatal("oversized document accepted")
				}
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("oversized document was published: %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Size() != int64(size) {
				t.Fatalf("persisted size mismatch: %v, %v", info, err)
			}
			if _, err := store.List(); err != nil {
				t.Fatal("accepted document cannot be read", err)
			}
		})
	}
}
