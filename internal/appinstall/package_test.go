package appinstall

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReceiptNeverAcceptsDeletionPathsOrDuplicateFolders(t *testing.T) {
	r := Receipt{Schema: Schema, Generation: 234, Revision: strings.Repeat("a", 40), Folders: []string{strings.Repeat("b", 32)}}
	data, _ := json.Marshal(r)
	if _, err := DecodeReceipt(data); err != nil {
		t.Fatal(err)
	}
	for _, folders := range [][]string{{"../identity"}, {"C:\\Windows"}, {strings.Repeat("b", 32), strings.Repeat("b", 32)}, {strings.Repeat("B", 32)}, nil} {
		r.Folders = folders
		data, _ = json.Marshal(r)
		if _, err := DecodeReceipt(data); err == nil {
			t.Fatal("unsafe deletion receipt accepted", folders)
		}
	}
	if _, err := Read([]byte("not a product archive")); err == nil {
		t.Fatal("unverified archive accepted")
	}
}
