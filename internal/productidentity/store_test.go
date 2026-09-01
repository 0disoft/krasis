package productidentity

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
)

func TestResolveWindowsPathsSeparatesDataClasses(t *testing.T) {
	paths, err := ResolveWindowsPaths(`C:\Users\zero\AppData\Local`)
	if err != nil {
		t.Fatal(err)
	}
	if paths.Root != `C:\Users\zero\AppData\Local\Krasis` || paths.Identity == paths.Trust || paths.Trust == paths.Diagnostics {
		t.Fatalf("paths = %+v", paths)
	}
}

func TestStoreCreatesLoadsAndRotatesIdentity(t *testing.T) {
	store := testStore(t)
	createdAt := time.Date(2026, time.September, 1, 1, 2, 3, 0, time.UTC)
	first, created, err := store.OpenOrCreate(createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if !created || first.Public().Generation != 1 || first.Public().Fingerprint == "" {
		t.Fatalf("first = %+v, created = %t", first.Public(), created)
	}
	loaded, created, err := store.OpenOrCreate(createdAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if created || loaded.Public() != first.Public() {
		t.Fatalf("loaded = %+v, created = %t", loaded.Public(), created)
	}
	message := []byte("pairing transcript")
	signature, err := loaded.Sign(message)
	if err != nil || len(signature) == 0 {
		t.Fatalf("Sign() signature = %x, error = %v", signature, err)
	}
	rotatedAt := createdAt.Add(24 * time.Hour)
	rotated, err := store.Rotate(rotatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Public().Generation != 2 || rotated.Public().Fingerprint == first.Public().Fingerprint || !rotated.Public().CreatedAt.Equal(rotatedAt) {
		t.Fatalf("rotated = %+v", rotated.Public())
	}
}

func TestStoreRejectsCorruptionUnknownFieldsAndKeyMismatch(t *testing.T) {
	store := testStore(t)
	identity, _, err := store.OpenOrCreate(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.directory, identityFile)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(original, &document); err != nil {
		t.Fatal(err)
	}
	public := append([]byte(nil), identity.private.Public().(ed25519.PublicKey)...)
	public[0] ^= 0xff
	document["publicKey"] = public
	mismatched, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{
		[]byte(`{"schemaVersion":`),
		append(bytes.TrimSpace(original), []byte(" {}")...),
		bytes.Replace(original, []byte(`"generation": 1`), []byte(`"generation": 1, "unknown": true`), 1),
		mismatched,
		[]byte(strings.Repeat("x", maxStoreBytes+1)),
	} {
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(); err == nil {
			t.Fatalf("Load() accepted payload %q", payload[:min(len(payload), 80)])
		}
	}
}

func TestStoreRejectsInsecureAndReparseBackedDirectory(t *testing.T) {
	parent := protectedParent(t)
	directory := filepath.Join(parent, "identity")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.OpenOrCreate(time.Now().UTC()); err == nil {
		t.Fatal("OpenOrCreate() accepted an insecure directory")
	}

	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "identity-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	linked, err := New(link)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := linked.OpenOrCreate(time.Now().UTC()); err == nil {
		t.Fatal("OpenOrCreate() accepted a reparse-backed directory")
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(filepath.Join(protectedParent(t), "identity"))
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
