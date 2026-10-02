package peerupdate

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestClipboardStatusSeparatesDeliveryFromApplication(t *testing.T) {
	m := testManager(t)
	m.generation = 121
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SyncClipboardPayload(peer, nil, "")
	data := testClipboardZip(t, "private-file.txt", false, "private content")
	if err := m.clipboardReceivePayload(peer, id, "files", bytes.NewReader(data), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	if s := m.ClipboardTransferStatus(); s.State != "READY" {
		t.Fatalf("premature application: %+v", s)
	}
	if err := m.ReportClipboard(peer, id, "files", "apply_failed"); err != nil {
		t.Fatal(err)
	}
	if s := m.ClipboardTransferStatus(); s.State != "FAILED" || s.Code != "apply_failed" {
		t.Fatalf("missing failure: %+v", s)
	}
	_, _ = m.SyncClipboardPayload(peer, nil, id)
	if s := m.ClipboardTransferStatus(); s.State != "APPLIED" {
		t.Fatalf("ack missing: %+v", s)
	}
	// A repeated GET/POST of an already-seen copy cannot regress the final status.
	_ = m.clipboardReceivePayload(peer, id, "files", bytes.NewReader(data), func() bool { return true })
	if s := m.ClipboardTransferStatus(); s.State != "APPLIED" {
		t.Fatalf("duplicate regressed: %+v", s)
	}
	_, _ = m.SyncClipboardPayload(peer, &ClipboardValue{Format: "files", Data: data}, "")
	id = m.clipboard.out.ID
	m.clipboardProgress(peer, id, "SEND", "files", 10, int64(len(data)))
	m.clipboardSent(peer, id)
	s := m.ClipboardTransferStatus()
	if s.State != "SENT" || s.Bytes != s.Total {
		t.Fatalf("sender result: %+v", s)
	}
	encoded, _ := json.Marshal(s)
	if bytes.Contains(encoded, []byte("private")) || bytes.Contains(encoded, []byte(id)) {
		t.Fatal("metadata exposed content or ID")
	}
}

func TestClipboardStatusRejectsStaleAndUnboundedReports(t *testing.T) {
	m := testManager(t)
	m.generation = 121
	peer := strings.Repeat("a", 64)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	m.clipboardProgress(peer, strings.Repeat("c", 32), "RECEIVE", "files", MaxClipboardFileBytes+1, 0)
	m.clipboardReceiveFailed(peer, strings.Repeat("c", 32))
	if s := m.ClipboardTransferStatus(); s.Bytes != MaxClipboardFileBytes || s.Code != "receive_failed" {
		t.Fatalf("over-limit failure metadata: %+v", s)
	}
	if m.ReportClipboard(strings.Repeat("b", 64), "", "files", "size_limit") == nil || m.ReportClipboard(peer, "", "files", "private native error") == nil {
		t.Fatal("unbounded or stale report accepted")
	}
	if err := m.ReportClipboard(peer, "", "files", "size_limit"); err != nil {
		t.Fatal(err)
	}
	if s := m.ClipboardTransferStatus(); s.Code != "size_limit" {
		t.Fatalf("limit missing: %+v", s)
	}
	data := testClipboardZip(t, "file.txt", false, "data")
	_, _ = m.SyncClipboardPayload(peer, &ClipboardValue{Format: "files", Data: data}, "")
	m.clipboard.out.expires = time.Now().Add(-time.Second)
	if s := m.ClipboardTransferStatus(); s.State != "FAILED" || s.Code != "expired" {
		t.Fatalf("expiry missing: %+v", s)
	}
	_ = m.SetClipboardPeer("")
	if s := m.ClipboardTransferStatus(); s.State != "" {
		t.Fatalf("disabled left stale metadata: %+v", s)
	}
}
