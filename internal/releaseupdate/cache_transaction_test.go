package releaseupdate

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

func packagedReadyArchive(t *testing.T) []byte {
	t.Helper()
	folder := os.Getenv("KRASIS_RELEASE_SMOKE")
	if folder == "" {
		t.Skip("requires locally built product package")
	}
	dir, err := securefs.OpenReadDirectory(folder)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	archive, err := updatebundle.Pack(dir)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

func readyEnvelope(t *testing.T, archive []byte, priv ed25519.PrivateKey) ([]byte, Manifest) {
	t.Helper()
	b, err := updatebundle.Read(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	meta := b.Metadata()
	m := manifest()
	m.Generation, m.Revision, m.Size = meta.Generation, meta.Revision, int64(len(archive))
	hash := sha256.Sum256(archive)
	m.SHA256 = hex.EncodeToString(hash[:])
	m.URL = fmt.Sprintf("https://github.com/0disoft/krasis/releases/download/g%d/krasis-g%d-windows-amd64.zip", meta.Generation, meta.Generation)
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(Envelope{base64.StdEncoding.EncodeToString(data), base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data))})
	if err != nil {
		t.Fatal(err)
	}
	return envelope, m
}

func TestReadyPublicationFailurePreservesPreviousProduct(t *testing.T) {
	first := packagedReadyArchive(t)
	// A ZIP comment changes the signed archive digest without altering product
	// files. Both bundles pass the real EXE and manifest validation.
	if len(first) < 22 || !bytes.Equal(first[len(first)-22:len(first)-18], []byte{'P', 'K', 5, 6}) || binary.LittleEndian.Uint16(first[len(first)-2:]) != 0 {
		t.Fatal("expected comment-free ZIP")
	}
	second := append([]byte(nil), first...)
	binary.LittleEndian.PutUint16(second[len(second)-2:], 4)
	second = append(second, "next"...)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString(pub)
	oldEnvelope, old := readyEnvelope(t, first, priv)
	newEnvelope, next := readyEnvelope(t, second, priv)
	for _, legacy := range []bool{false, true} {
		for _, failReceipt := range []bool{false, true} {
			t.Run(fmt.Sprintf("legacy_%t_receipt_%t", legacy, failReceipt), func(t *testing.T) {
				base := t.TempDir()
				oldPath, _, err := SaveReady(base, oldEnvelope, first, key)
				if err != nil {
					t.Fatal(err)
				}
				dir, err := productidentity.PrepareLaunch(base)
				if err != nil {
					t.Fatal(err)
				}
				defer dir.Close()
				if legacy {
					if err = writeCache(dir, "release-ready.zip", first); err != nil {
						t.Fatal(err)
					}
					if err = dir.Remove(archiveName(old)); err != nil {
						t.Fatal(err)
					}
				}
				before, err := readCache(dir, "release-ready.json", maxReceiptBytes)
				if err != nil {
					t.Fatal(err)
				}
				fault := errors.New("injected storage failure")
				write := func(d *securefs.Directory, name string, data []byte) error {
					if (name == "release-ready.json") == failReceipt {
						return fault
					}
					return writeCache(d, name, data)
				}
				if _, _, err = saveReady(base, newEnvelope, second, key, write); !errors.Is(err, fault) {
					t.Fatal("storage failure not returned", err)
				}
				after, err := readCache(dir, "release-ready.json", maxReceiptBytes)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("previous receipt changed", err)
				}
				restored, got, err := RestoreReady(base, key, old.Generation-1)
				if err != nil || restored != oldPath || got != old {
					t.Fatal("previous product no longer restorable", got, err)
				}
				// A retry must publish the new complete pair, including when the
				// previous failed attempt left its archive and stage on disk.
				newPath, _, err := SaveReady(base, newEnvelope, second, key)
				if err != nil {
					t.Fatal("retry failed", err)
				}
				restored, got, err = RestoreReady(base, key, old.Generation-1)
				if err != nil || restored != newPath || got != next || newPath == oldPath {
					t.Fatal("new product not restored", got, err)
				}
				// An old compatible archive must not hide corruption of the
				// digest-named current archive.
				if err = writeCache(dir, "release-ready.zip", second); err != nil {
					t.Fatal(err)
				}
				if err = writeCache(dir, archiveName(next), []byte("changed")); err != nil {
					t.Fatal(err)
				}
				if _, _, err = RestoreReady(base, key, old.Generation-1); !errors.Is(err, ErrInvalid) {
					t.Fatal("corrupt new archive fell back to legacy", err)
				}
			})
		}
	}
}
