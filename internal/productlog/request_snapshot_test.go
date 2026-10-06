package productlog

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestCollectionSnapshotsSurviveLaterExports(t *testing.T) {
	base := t.TempDir()
	id, next, peer := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 64)
	if err := SaveCleanup(base, CleanupSummary{Time: "2026-10-07T00:00:00Z", Code: "failed", Removed: 1}); err != nil {
		t.Fatal(err)
	}
	local, err := ExportForRequest(base, id)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := SaveRemoteSupportForRequest(base, peer, id, before)
	if err != nil {
		t.Fatal(err)
	}
	remoteBefore, err := os.ReadFile(remote)
	if err != nil {
		t.Fatal(err)
	}
	if err = SaveCleanup(base, CleanupSummary{Time: "2026-10-07T00:01:00Z", Code: "completed", Removed: 2}); err != nil {
		t.Fatal(err)
	}
	latest, err := Export(base)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(latest)
	if err != nil || bytes.Equal(before, after) {
		t.Fatal("fixture did not change", err)
	}
	newLocal, err := ExportForRequest(base, next)
	if err != nil || newLocal == local {
		t.Fatal(newLocal, err)
	}
	if _, err = SaveRemoteSupport(base, peer, after); err != nil {
		t.Fatal(err)
	}
	newRemote, err := SaveRemoteSupportForRequest(base, peer, next, after)
	if err != nil || newRemote == remote {
		t.Fatal(newRemote, err)
	}
	if repeated, err := ExportForRequest(base, id); err != nil || repeated != local {
		t.Fatal(repeated, err)
	}
	if repeated, err := SaveRemoteSupportForRequest(base, peer, id, before); err != nil || repeated != remote {
		t.Fatal(repeated, err)
	}
	if _, err = SaveRemoteSupportForRequest(base, peer, id, after); err == nil {
		t.Fatal("conflicting response replaced original snapshot")
	}
	for path, want := range map[string][]byte{local: before, remote: remoteBefore} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("collection snapshot changed", path, err)
		}
	}
	for _, bad := range []string{"", "../bad", strings.Repeat("A", 32)} {
		if _, err = ExportForRequest(base, bad); err == nil {
			t.Fatal("invalid request ID accepted")
		}
		if _, err = SaveRemoteSupportForRequest(base, peer, bad, before); err == nil {
			t.Fatal("invalid remote request ID accepted")
		}
	}
}
