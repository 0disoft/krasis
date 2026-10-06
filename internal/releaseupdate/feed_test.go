package releaseupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func signed(t *testing.T, m Manifest) ([]byte, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(m)
	e, _ := json.Marshal(Envelope{base64.StdEncoding.EncodeToString(data), base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data))})
	return e, base64.StdEncoding.EncodeToString(pub)
}
func manifest() Manifest {
	return Manifest{"krasis.release/v1", 184, strings.Repeat("a", 40), "windows/amd64", "https://github.com/0disoft/krasis/releases/download/g184/krasis-g184-windows-amd64.zip", strings.Repeat("b", 64), 1024}
}
func TestVerifyRejectsTamperingAndCrossRepositoryDownloads(t *testing.T) {
	m := manifest()
	data, key := signed(t, m)
	got, err := Verify(data, key)
	if err != nil || got != m {
		t.Fatalf("%v %v", got, err)
	}
	var e Envelope
	_ = json.Unmarshal(data, &e)
	e.Payload = base64.StdEncoding.EncodeToString([]byte(`{}`))
	tampered, _ := json.Marshal(e)
	if _, err = Verify(tampered, key); err == nil {
		t.Fatal("tampered payload accepted")
	}
	for _, mutate := range []func(*Manifest){func(m *Manifest) { m.URL = "https://github.com/other/repo/a.zip" }, func(m *Manifest) { m.Size = 1 << 30 }, func(m *Manifest) { m.Platform = "linux/amd64" }, func(m *Manifest) { m.Revision = "../path" }} {
		m := manifest()
		mutate(&m)
		d, k := signed(t, m)
		if _, err = Verify(d, k); err == nil {
			t.Fatal("invalid signed manifest accepted")
		}
	}
	if _, err = Verify(data, ""); !errors.Is(err, ErrUnconfigured) {
		t.Fatal(err)
	}
}
