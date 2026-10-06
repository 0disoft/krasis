package releaseupdate

import (
	"encoding/base64"
	"testing"
)

func TestEmbeddedKeyCheckRejectsMissingMalformedAndForeignKeys(t *testing.T) {
	old := PublicKey
	t.Cleanup(func() { PublicKey = old })
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	PublicKey = key
	if !MatchEmbeddedKey(key) {
		t.Fatal("linked key rejected")
	}
	for _, value := range []string{"", "bad", base64.StdEncoding.EncodeToString(make([]byte, 31))} {
		if MatchEmbeddedKey(value) {
			t.Fatal("invalid key accepted")
		}
	}
	PublicKey = ""
	if MatchEmbeddedKey(key) {
		t.Fatal("unconfigured image accepted")
	}
}
