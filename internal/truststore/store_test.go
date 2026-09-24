package truststore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/securefs"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/transport"
)

func TestStoreImportListRevokeAndPersist(t *testing.T) {
	parent := protectedParent(t)
	directory := filepath.Join(parent, "trust")
	store, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	peer := testPeer()
	createdAt := time.Date(2026, time.July, 13, 1, 2, 3, 0, time.UTC)
	record, err := store.Import(peer, "집 노트북", true, createdAt)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if record.Fingerprint != peer.CertificateSHA256 || !record.AllowInboundControl {
		t.Fatalf("record = %+v", record)
	}
	document, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if document.SchemaVersion != SchemaVersion || document.Revision != 1 || len(document.Peers) != 1 {
		t.Fatalf("document = %+v", document)
	}
	revokedAt := createdAt.Add(time.Hour)
	revoked, err := store.Revoke(peer.CertificateSHA256, revokedAt)
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if revoked.RevokedAt == nil || !revoked.RevokedAt.Equal(revokedAt) {
		t.Fatalf("revoked = %+v", revoked)
	}
	reopened, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	document, err = reopened.List()
	if err != nil {
		t.Fatalf("reopened List() error = %v", err)
	}
	if document.Revision != 2 || document.Peers[0].RevokedAt == nil {
		t.Fatalf("reopened document = %+v", document)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".trust-") && entry.Name() != lockFilename {
			t.Fatalf("temporary file remained: %s", entry.Name())
		}
	}
}

func TestStoreRejectsDuplicateAndRepeatedRevocation(t *testing.T) {
	store := mustStore(t)
	peer := testPeer()
	now := time.Now().UTC()
	if _, err := store.Import(peer, "peer", true, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(peer, "peer again", true, now.Add(time.Second)); !errors.Is(err, ErrPeerExists) {
		t.Fatalf("duplicate import error = %v", err)
	}
	if _, err := store.Revoke(peer.CertificateSHA256, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Revoke(peer.CertificateSHA256, now.Add(2*time.Minute)); !errors.Is(err, ErrPeerRevoked) {
		t.Fatalf("repeated revoke error = %v", err)
	}
}

func TestStoreUpdatesAliasAndCapabilityAtomically(t *testing.T) {
	store := mustStore(t)
	peer := testPeer()
	createdAt := time.Date(2026, time.July, 13, 1, 2, 3, 0, time.UTC)
	if _, err := store.Import(peer, "old alias", false, createdAt); err != nil {
		t.Fatal(err)
	}
	updatedAt := createdAt.Add(time.Minute)
	updated, err := store.Update(peer.CertificateSHA256, "new alias", true, updatedAt)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Alias != "new alias" || !updated.AllowInboundControl || !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("updated = %+v", updated)
	}
	document, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if document.Revision != 2 || len(document.Peers) != 1 || document.Peers[0] != updated {
		t.Fatalf("document = %+v", document)
	}
}

func TestStoreReplacesPeerInOneRevision(t *testing.T) {
	store := mustStore(t)
	oldPeer := testPeer()
	createdAt := time.Date(2026, time.July, 13, 1, 2, 3, 0, time.UTC)
	if _, err := store.Import(oldPeer, "old device", true, createdAt); err != nil {
		t.Fatal(err)
	}
	replacement := transport.AuthenticatedPeerContext{
		CertificateSHA256: strings.Repeat("b", 64),
		Role:              transport.AuthenticatedPeerConnector,
	}
	replacedAt := createdAt.Add(time.Hour)
	revoked, added, err := store.Replace(oldPeer.CertificateSHA256, replacement, "replacement device", false, replacedAt)
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if revoked.RevokedAt == nil || !revoked.RevokedAt.Equal(replacedAt) {
		t.Fatalf("revoked = %+v", revoked)
	}
	if added.Fingerprint != replacement.CertificateSHA256 || added.AllowInboundControl {
		t.Fatalf("added = %+v", added)
	}
	document, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if document.Revision != 2 || len(document.Peers) != 2 {
		t.Fatalf("document = %+v", document)
	}
}

func TestStoreRejectsChangesToRevokedOrDuplicateReplacement(t *testing.T) {
	store := mustStore(t)
	now := time.Date(2026, time.July, 13, 1, 2, 3, 0, time.UTC)
	first := testPeer()
	second := transport.AuthenticatedPeerContext{
		CertificateSHA256: strings.Repeat("b", 64),
		Role:              transport.AuthenticatedPeerConnector,
	}
	if _, err := store.Import(first, "first", true, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(second, "second", true, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Revoke(first.CertificateSHA256, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(first.CertificateSHA256, "revived", true, now.Add(2*time.Minute)); !errors.Is(err, ErrPeerRevoked) {
		t.Fatalf("Update() error = %v", err)
	}
	if _, _, err := store.Replace(first.CertificateSHA256, second, "duplicate", true, now.Add(3*time.Minute)); !errors.Is(err, ErrPeerExists) {
		t.Fatalf("Replace() error = %v", err)
	}
}

func TestStoreRejectsCorruptOrUnknownJSONWithoutFallback(t *testing.T) {
	store := mustStore(t)
	if _, err := store.Import(testPeer(), "peer", true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.directory, storeFilename)
	for _, payload := range []string{
		`{"schemaVersion":"krasis.trust-store/v1","revision":1,"updatedAt":"2026-07-13T00:00:00Z","peers":[],"unknown":true}`,
		`{"schemaVersion":`,
		`{"schemaVersion":"krasis.trust-store/v1","revision":1,"updatedAt":"2026-07-13T00:00:00Z","peers":[]} {}`,
		strings.Repeat(" ", maxStoreBytes+1),
	} {
		if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.List(); err == nil {
			t.Fatalf("List() accepted payload %q", payload)
		}
	}
}

func TestStoreRejectsExistingInsecureDirectory(t *testing.T) {
	directory := filepath.Join(protectedParent(t), "trust")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(testPeer(), "peer", true, time.Now().UTC()); err == nil {
		t.Fatal("Import() accepted an insecure existing directory")
	}
}

func TestStoreRejectsConcurrentMutationLock(t *testing.T) {
	store := mustStore(t)
	if _, err := store.Import(testPeer(), "peer", true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	directory, err := securefs.OpenDirectory(store.directory)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	lock, _, err := openLockFile(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	unlock, err := acquireFileLock(lock, false)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := store.List(); !errors.Is(err, ErrStoreBusy) {
		t.Fatalf("List() error = %v, want ErrStoreBusy", err)
	}
}

func TestFileLockAllowsReadersButExcludesWriter(t *testing.T) {
	store := mustStore(t)
	if _, err := store.Import(testPeer(), "peer", true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	directory, err := securefs.OpenDirectory(store.directory)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	first, _, err := openLockFile(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, _, err := openLockFile(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	writer, _, err := openLockFile(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	unlockFirst, err := acquireFileLock(first, true)
	if err != nil {
		t.Fatal(err)
	}
	unlockSecond, err := acquireFileLock(second, true)
	if err != nil {
		_ = unlockFirst()
		t.Fatal(err)
	}
	if _, err := acquireFileLock(writer, false); !errors.Is(err, ErrStoreBusy) {
		_ = unlockSecond()
		_ = unlockFirst()
		t.Fatalf("writer lock error = %v, want ErrStoreBusy", err)
	}
	if err := unlockSecond(); err != nil {
		t.Fatal(err)
	}
	if err := unlockFirst(); err != nil {
		t.Fatal(err)
	}
	unlockWriter, err := acquireFileLock(writer, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := unlockWriter(); err != nil {
		t.Fatal(err)
	}
}

func TestPeerRecordRejectsUnsafeAlias(t *testing.T) {
	record := PeerRecord{
		Fingerprint: testPeer().CertificateSHA256, Role: transport.AuthenticatedPeerConnector,
		Alias: "peer\nforged", CreatedAt: time.Now().UTC(),
	}
	if err := record.Validate(); err == nil {
		t.Fatal("PeerRecord.Validate() accepted a control character")
	}
}

func mustStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(filepath.Join(protectedParent(t), "trust"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func protectedParent(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	if _, err := peercredentials.ProtectSecurityDirectory(parent); err != nil {
		t.Fatal(err)
	}
	return parent
}

func testPeer() transport.AuthenticatedPeerContext {
	return transport.AuthenticatedPeerContext{
		CertificateSHA256: strings.Repeat("a", 64),
		Role:              transport.AuthenticatedPeerConnector,
	}
}

func TestSecurityHelpersProtectStoreFiles(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "trust")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := peercredentials.ProtectSecurityDirectory(directory); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "trust.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := peercredentials.ProtectSecurityFile(path); err != nil {
		t.Fatal(err)
	}
	if err := peercredentials.ValidateSecurityFile(path); err != nil {
		t.Fatal(err)
	}
}
