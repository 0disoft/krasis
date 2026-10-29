package appinstall

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestReceiptHistoryCapacityFitsUnchangedByteLimit(t *testing.T) {
	r := Receipt{Schema: Schema, Generation: 501, Revision: strings.Repeat("a", 40)}
	for i := range maxInstallFolders {
		r.Folders = append(r.Folders, fmt.Sprintf("%032x", i))
	}
	data, err := json.Marshal(r)
	if err != nil || len(data) > 8192 {
		t.Fatal("bounded history no longer fits existing receipt reads", len(data), err)
	}
	if got, err := DecodeReceipt(data); err != nil || len(got.Folders) != maxInstallFolders {
		t.Fatal("full bounded history rejected", err)
	}
	r.Folders = append(r.Folders, fmt.Sprintf("%032x", maxInstallFolders))
	data, _ = json.Marshal(r)
	if _, err := DecodeReceipt(data); err == nil {
		t.Fatal("unbounded install history accepted")
	}
}

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
