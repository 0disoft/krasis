package productlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productidentity"
)

func TestCollectionReceiptRestoresOnlyItsActualPair(t *testing.T) {
	base := t.TempDir()
	id, peer := strings.Repeat("a", 32), strings.Repeat("b", 64)
	local, err := ExportForRequest(base, id)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	r := CollectionReceipt{Schema: "krasis.diagnostic-collection/v1", ID: id, Peer: peer, Expires: time.Now().UTC().Add(time.Minute), State: "PENDING", Code: "pending", LocalSaved: true}
	if err := SaveCollectionReceipt(base, r); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadCollectionReceipt(base); err != nil || got == nil || *got != r {
		t.Fatal(got, err)
	}
	if got, err := CollectionLocalPath(base); err != nil || got != local {
		t.Fatal(got, err)
	}
	remote, err := SaveRemoteSupportForRequest(base, peer, id, data)
	if err != nil {
		t.Fatal(err)
	}
	r.State, r.Code, r.PeerSaved = "RECEIVED", "collected", true
	if err := SaveCollectionReceipt(base, r); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadCollectionReceipt(base); err != nil || got == nil || !got.PeerSaved {
		t.Fatal(got, err)
	}
	if err := os.Remove(remote); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCollectionReceipt(base); err == nil {
		t.Fatal("missing peer file restored as complete")
	}
}

func TestCollectionReceiptRejectsPathsMalformedAndTrailingData(t *testing.T) {
	base := t.TempDir()
	r := CollectionReceipt{Schema: "krasis.diagnostic-collection/v1", State: "SAVING", Code: "saving"}
	if err := SaveCollectionReceipt(base, r); err != nil {
		t.Fatal(err)
	}
	paths, _ := productidentity.ResolveWindowsPaths(base)
	path := filepath.Join(paths.Diagnostics, CollectionReceiptName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tail := range []string{" {}", " private"} {
		if err := os.WriteFile(path, append(append([]byte{}, before...), []byte(tail)...), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadCollectionReceipt(base); err == nil {
			t.Fatal("accepted trailing receipt data")
		}
	}
	r.ID = "../private"
	if err := SaveCollectionReceipt(base, r); err == nil {
		t.Fatal("accepted arbitrary file reference")
	}
}
