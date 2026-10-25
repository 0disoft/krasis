package releaseupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

func TestReadyCacheRejectsUnsignedAndChangedArchive(t *testing.T) {
	base := t.TempDir()
	m := manifest()
	envelope, key := signed(t, m)
	if _, _, err := SaveReady(base, envelope, []byte("changed"), key); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(readyReceipt{Envelope: envelope, Folders: []string{m.SHA256[:32]}})
	if err = writeCache(dir, "release-ready.json", raw); err != nil {
		t.Fatal(err)
	}
	if err = writeCache(dir, "release-ready.zip", []byte("changed")); err != nil {
		t.Fatal(err)
	}
	dir.Close()
	if _, _, err = RestoreReady(base, key, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	_, otherKey := signed(t, m)
	if _, _, err = RestoreReady(base, otherKey, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestReadyPruneKeepsTwoPublicStagesAndIgnoresPeerStages(t *testing.T) {
	base := t.TempDir()
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	paths, _ := productidentity.ResolveWindowsPaths(base)
	parent := filepath.Join(paths.Root, "updates")
	if err = os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = peercredentials.ProtectSecurityDirectory(parent); err != nil {
		t.Fatal(err)
	}
	ids := []string{strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)}
	for _, id := range append(ids, strings.Repeat("d", 32)) {
		if err = os.Mkdir(filepath.Join(parent, id), 0700); err != nil {
			t.Fatal(err)
		}
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	history := map[string][]byte{}
	for _, id := range ids {
		m := manifest()
		m.SHA256 = id + strings.Repeat("e", 32)
		data, _ := json.Marshal(m)
		envelope, _ := json.Marshal(Envelope{base64.StdEncoding.EncodeToString(data), base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data))})
		history[id] = envelope
		if err = writeCache(dir, archiveName(m), []byte("archive retention fixture")); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(readyReceipt{Folders: ids, History: history})
	if err = writeCache(dir, "release-ready.json", raw); err != nil {
		t.Fatal(err)
	}
	if err = PruneReady(base, base64.StdEncoding.EncodeToString(pub)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{ids[0], ids[1], strings.Repeat("d", 32)} {
		if _, err = os.Stat(filepath.Join(parent, id)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = os.Stat(filepath.Join(parent, ids[2])); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old public folder retained", err)
	}
	for i, id := range ids {
		m, err := Verify(history[id], base64.StdEncoding.EncodeToString(pub))
		if err != nil {
			t.Fatal(err)
		}
		f, err := dir.Open(archiveName(m))
		if f != nil {
			f.Close()
		}
		if (i < 2 && err != nil) || (i == 2 && !errors.Is(err, os.ErrNotExist)) {
			t.Fatal("archive retention does not match stages", id, err)
		}
	}
}

func TestReadyCacheRoundTripWithPackagedProduct(t *testing.T) {
	folder := os.Getenv("KRASIS_RELEASE_SMOKE")
	if folder == "" {
		t.Skip("requires locally built product package")
	}
	guard, err := securefs.OpenReadDirectory(folder)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := updatebundle.Pack(guard)
	guard.Close()
	if err != nil {
		t.Fatal(err)
	}
	b, err := updatebundle.Read(strings.NewReader(string(archive)), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	meta := b.Metadata()
	m := manifest()
	m.Generation = meta.Generation
	m.Revision = meta.Revision
	m.Size = int64(len(archive))
	hash := sha256.Sum256(archive)
	m.SHA256 = hex.EncodeToString(hash[:])
	m.URL = fmt.Sprintf("https://github.com/0disoft/krasis/releases/download/g%d/krasis-g%d-windows-amd64.zip", meta.Generation, meta.Generation)
	envelope, key := signed(t, m)
	base := t.TempDir()
	c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: m.Size, Body: io.NopCloser(bytes.NewReader(archive))}, nil
	})}
	downloaded, err := DownloadArchiveBytes(context.Background(), c, m)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := SaveReady(base, envelope, downloaded, key)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := RestoreReady(base, key, m.Generation-1)
	if err != nil || first != second {
		t.Fatal("ready not restored", err)
	}
	third, _, err := SaveReady(base, envelope, archive, key)
	if err != nil || first != third {
		t.Fatal("duplicate stage", err)
	}
	if err = os.WriteFile(filepath.Join(first, "START-HERE.md"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = RestoreReady(base, key, m.Generation-1); err == nil {
		t.Fatal("changed staged file accepted")
	}
}
