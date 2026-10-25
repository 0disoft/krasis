package releaseupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

func TestDownloadArchiveBytesRequiresBoundedSizeAndMatchingDigest(t *testing.T) {
	m := manifest()
	m.Size = 3
	digest := sha256.Sum256([]byte("abc"))
	m.SHA256 = hex.EncodeToString(digest[:])
	for _, body := range []string{"abc", "abd", "ab", "abcd"} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, ContentLength: -1, Body: io.NopCloser(bytes.NewBufferString(body))}, nil
			})}
			data, err := DownloadArchiveBytes(context.Background(), c, m)
			if calls != 1 {
				t.Fatal("unexpected network calls", calls)
			}
			if body == "abc" {
				if err != nil || string(data) != body {
					t.Fatal("matching bytes rejected", err)
				}
			} else if !errors.Is(err, ErrInvalid) || data != nil {
				t.Fatal("unverified bytes returned", err)
			}
		})
	}
	c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid size contacted network")
		return nil, nil
	})}
	for _, size := range []int64{-1, 0, updatebundle.MaxArchiveBytes + 1, 1<<63 - 1} {
		m.Size = size
		if data, err := DownloadArchiveBytes(context.Background(), c, m); !errors.Is(err, ErrInvalid) || data != nil {
			t.Fatal("invalid size accepted", size, err)
		}
	}
}

func TestDownloadedBytesCannotPublishMalformedBundle(t *testing.T) {
	archive := []byte("digest matches but this is not a ZIP")
	m := manifest()
	m.Size = int64(len(archive))
	digest := sha256.Sum256(archive)
	m.SHA256 = hex.EncodeToString(digest[:])
	envelope, key := signed(t, m)
	c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: int64(len(archive)), Body: io.NopCloser(bytes.NewReader(archive))}, nil
	})}
	data, err := DownloadArchiveBytes(context.Background(), c, m)
	if err != nil {
		t.Fatal("download unnecessarily expanded bundle", err)
	}
	base := t.TempDir()
	if path, _, err := SaveReady(base, envelope, data, key); err == nil || path != "" {
		t.Fatal("malformed bundle authorized staging", path, err)
	}
	entries, err := os.ReadDir(base)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid archive caused cache writes", entries, err)
	}
	if bundle, _, err := DownloadArchive(context.Background(), c, m); err == nil || bundle != nil {
		t.Fatal("existing full download contract weakened", err)
	}
}

// Both paths still call the unchanged SaveReady afterwards; compare only the
// download work that differs, with an actual packaged product as fixture data.
func BenchmarkDownloadBeforeSaveReady(b *testing.B) {
	folder := os.Getenv("KRASIS_RELEASE_SMOKE")
	if folder == "" {
		b.Skip("requires locally built product package")
	}
	dir, err := securefs.OpenReadDirectory(folder)
	if err != nil {
		b.Fatal(err)
	}
	archive, err := updatebundle.Pack(dir)
	dir.Close()
	if err != nil {
		b.Fatal(err)
	}
	bundle, err := updatebundle.Read(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		b.Fatal(err)
	}
	meta := bundle.Metadata()
	m := manifest()
	m.Generation, m.Revision, m.Size = meta.Generation, meta.Revision, int64(len(archive))
	digest := sha256.Sum256(archive)
	m.SHA256 = hex.EncodeToString(digest[:])
	c := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: m.Size, Body: io.NopCloser(bytes.NewReader(archive))}, nil
	})}
	for _, full := range []bool{true, false} {
		name := "bytes_only"
		if full {
			name = "full_bundle_discarded"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(archive)))
			for b.Loop() {
				if full {
					_, _, err = DownloadArchive(context.Background(), c, m)
				} else {
					_, err = DownloadArchiveBytes(context.Background(), c, m)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
