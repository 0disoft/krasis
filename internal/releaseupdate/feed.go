// Package releaseupdate accepts only publisher-signed, bounded GitHub releases.
package releaseupdate

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/0disoft/krasis/internal/updatebundle"
	"io"
)

const FeedURL = "https://github.com/0disoft/krasis/releases/latest/download/latest.json"

// PublicKey is embedded at release build time; absent configuration fails closed.
var PublicKey string
var ErrUnconfigured = errors.New("release signing key not configured")
var ErrInvalid = errors.New("invalid signed release")

// MatchEmbeddedKey is a read-only packaging check. Build flags are omitted from
// Go build metadata with -trimpath, so inspect the actual linked value instead.
func MatchEmbeddedKey(key string) bool {
	pub, err := base64.StdEncoding.DecodeString(key)
	return err == nil && len(pub) == ed25519.PublicKeySize && key == PublicKey
}

type Manifest struct {
	Schema     string `json:"schema"`
	Generation int    `json:"generation"`
	Revision   string `json:"revision"`
	Platform   string `json:"platform"`
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
}
type Envelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func decode(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return ErrInvalid
	}
	if d.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func Verify(data []byte, key string) (Manifest, error) {
	var m Manifest
	pub, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return m, ErrUnconfigured
	}
	if len(data) > 16384 {
		return m, ErrInvalid
	}
	var e Envelope
	if decode(data, &e) != nil {
		return m, ErrInvalid
	}
	payload, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return m, ErrInvalid
	}
	sig, err := base64.StdEncoding.DecodeString(e.Signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), payload, sig) {
		return m, ErrInvalid
	}
	if decode(payload, &m) != nil || m.Schema != "krasis.release/v1" || m.Platform != "windows/amd64" || m.Generation < 21 || m.Generation > 1000000 || m.Size <= 0 || m.Size > updatebundle.MaxArchiveBytes {
		return Manifest{}, ErrInvalid
	}
	if !lowerHex(m.Revision, 40) || !lowerHex(m.SHA256, 64) {
		return Manifest{}, ErrInvalid
	}
	want := fmt.Sprintf("https://github.com/0disoft/krasis/releases/download/g%d/krasis-g%d-windows-amd64.zip", m.Generation, m.Generation)
	if m.URL != want {
		return Manifest{}, ErrInvalid
	}
	return m, nil
}
func lowerHex(value string, n int) bool {
	if len(value) != n {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
