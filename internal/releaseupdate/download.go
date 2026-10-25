package releaseupdate

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func Client() *http.Client {
	return &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || r.URL.Scheme != "https" || r.URL.User != nil || r.URL.Port() != "" {
			return ErrInvalid
		}
		switch r.URL.Hostname() {
		case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
			return nil
		}
		return ErrInvalid
	}}
}
func fetch(ctx context.Context, c *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Krasis-release-updater")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode, RetryAfter: retryAfter(resp.Header.Get("Retry-After"), time.Now())}
	}
	if resp.ContentLength > limit {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrInvalid
	}
	return data, nil
}
func Check(ctx context.Context, c *http.Client, key string) (Manifest, error) {
	m, _, err := CheckSigned(ctx, c, key)
	return m, err
}
func CheckSigned(ctx context.Context, c *http.Client, key string) (Manifest, []byte, error) {
	// Validate configuration before any network request.
	if _, err := Verify(nil, key); errors.Is(err, ErrUnconfigured) {
		return Manifest{}, nil, err
	}
	data, err := fetch(ctx, c, FeedURL, 16384)
	if err != nil {
		return Manifest{}, nil, err
	}
	m, err := Verify(data, key)
	return m, data, err
}
func Download(ctx context.Context, c *http.Client, m Manifest) (*updatebundle.Bundle, error) {
	b, _, err := DownloadArchive(ctx, c, m)
	return b, err
}
func DownloadArchive(ctx context.Context, c *http.Client, m Manifest) (*updatebundle.Bundle, []byte, error) {
	data, err := fetch(ctx, c, m.URL, m.Size)
	if err != nil {
		return nil, nil, err
	}
	b, err := verifyArchive(data, m)
	return b, data, err
}

// DownloadArchiveBytes fetches bytes matching a previously verified manifest,
// without expanding the ZIP. The bytes do not authorize staging or installation:
// SaveReady must verify the signed envelope and complete bundle before publishing.
func DownloadArchiveBytes(ctx context.Context, c *http.Client, m Manifest) ([]byte, error) {
	if m.Size <= 0 || m.Size > updatebundle.MaxArchiveBytes {
		return nil, ErrInvalid
	}
	data, err := fetch(ctx, c, m.URL, m.Size)
	if err != nil {
		return nil, err
	}
	if !archiveMatchesManifest(data, m) {
		return nil, ErrInvalid
	}
	return data, nil
}

func archiveMatchesManifest(data []byte, m Manifest) bool {
	if int64(len(data)) != m.Size {
		return false
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]) == m.SHA256
}

func verifyArchive(data []byte, m Manifest) (*updatebundle.Bundle, error) {
	if !archiveMatchesManifest(data, m) {
		return nil, ErrInvalid
	}
	bundle, err := updatebundle.Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	meta := bundle.Metadata()
	if meta.Generation != m.Generation || meta.Revision != m.Revision || bundle.UIExecutable() != "krasis.exe" || !bundle.HasInputInstaller() {
		return nil, ErrInvalid
	}
	return bundle, nil
}
func Stage(base string, bundle *updatebundle.Bundle) (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return stage(base, bundle, hex.EncodeToString(id[:]))
}
func stage(base string, bundle *updatebundle.Bundle, name string) (string, error) {
	if bundle == nil {
		return "", ErrInvalid
	}
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return "", err
	}
	if _, err = peercredentials.ValidateSecurityDirectory(paths.Root); err != nil {
		return "", err
	}
	parent, err := securefs.OpenDirectory(paths.Root)
	if err != nil {
		return "", err
	}
	dir, err := parent.CreateDirectory("updates", 0700)
	if err == nil {
		err = peercredentials.ProtectSecurityDirectoryHandle(dir.Handle())
		dir.Close()
	} else if errors.Is(err, os.ErrExist) {
		_, err = peercredentials.ValidateSecurityDirectory(filepath.Join(paths.Root, "updates"))
	}
	parent.Close()
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(filepath.Join(paths.Root, "updates"))
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err = bundle.Stage(root, name); err != nil {
		return "", err
	}
	return filepath.Join(paths.Root, "updates", name), nil
}
