package peerupdate

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
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

func TestClipboardTextAndImageFailureReportsKeepPeerAndCopyAuthority(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"", "png"} {
		t.Run("format="+format, func(t *testing.T) {
			m := testManager(t)
			m.generation = 121
			peer, foreign, id := strings.Repeat("a", 64), strings.Repeat("c", 64), strings.Repeat("b", 32)
			if err := m.SetClipboardPeer(peer); err != nil {
				t.Fatal(err)
			}
			_, _ = m.SyncClipboardPayload(peer, nil, "")
			data := []byte("private clipboard content")
			if format == "png" {
				data = imageData.Bytes()
			}
			if err := m.clipboardReceivePayload(peer, id, format, bytes.NewReader(data), func() bool { return true }); err != nil {
				t.Fatal(err)
			}
			ready := m.ClipboardTransferStatus()
			for _, report := range []struct{ peer, id, format string }{
				{foreign, id, format}, {peer, strings.Repeat("d", 32), format}, {peer, id, "files"},
			} {
				if m.ReportClipboard(report.peer, report.id, report.format, "apply_failed") == nil || m.ClipboardTransferStatus() != ready {
					t.Fatal("foreign, obsolete or mismatched copy altered the status")
				}
			}
			if err := m.ReportClipboard(peer, id, format, "apply_failed"); err != nil {
				t.Fatal(err)
			}
			if status := m.ClipboardTransferStatus(); status.State != "FAILED" || status.Code != "apply_failed" || status.Format != format {
				t.Fatal("matching native failure missing", status)
			}
			_, _ = m.SyncClipboardPayload(peer, nil, id)
			if m.ReportClipboard(peer, id, format, "apply_failed") == nil || m.ClipboardTransferStatus().State != "APPLIED" {
				t.Fatal("obsolete failure regressed an acknowledged copy")
			}
			for _, code := range []string{"prepare_failed", "size_limit"} {
				if err := m.ReportClipboard(peer, "", format, code); err != nil {
					t.Fatal(err)
				}
				status := m.ClipboardTransferStatus()
				if status.Code != code || status.Format != format || status.Direction != "SEND" {
					t.Fatal("local failure missing", status)
				}
			}
			_ = m.SetClipboardPeer("")
			if m.ReportClipboard(peer, "", format, "prepare_failed") == nil || m.ReportClipboard("", "", format, "prepare_failed") == nil {
				t.Fatal("disabled sharing accepted failure metadata")
			}
		})
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
