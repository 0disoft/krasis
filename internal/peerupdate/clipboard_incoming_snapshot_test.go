package peerupdate

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestClipboardIncomingResultsKeepPrivatePayloads(t *testing.T) {
	m := testManager(t)
	m.generation = 72
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SyncClipboard(peer, nil, "")
	data := testClipboardZip(t, "file.txt", false, "original incoming copy")
	if err := m.clipboardReceivePayload(peer, id, "files", bytes.NewReader(data), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	first, err := m.SyncClipboard(peer, nil, "")
	if err != nil || first.ID != id || !bytes.Equal(first.Data, data) {
		t.Fatal("missing incoming copy", first.ID, err)
	}
	second, err := m.SyncClipboard(peer, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	first.Data[0] ^= 0xff
	if !bytes.Equal(second.Data, data) {
		t.Fatal("two API callers share a writable payload")
	}
	third, err := m.SyncClipboard(peer, nil, "")
	if err != nil || !bytes.Equal(third.Data, data) {
		t.Fatal("API caller mutation corrupted the queued copy", err)
	}
}

func TestClipboardIncomingSnapshotSurvivesQueueRetirement(t *testing.T) {
	for _, change := range []string{"ack", "new-local-copy", "new-remote-copy", "selection"} {
		t.Run(change, func(t *testing.T) {
			m := testManager(t)
			m.generation = 72
			peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
			if err := m.SetClipboardPeer(peer); err != nil {
				t.Fatal(err)
			}
			_, _ = m.SyncClipboard(peer, nil, "")
			data := testClipboardZip(t, "file.txt", false, "retired copy")
			if err := m.clipboardReceivePayload(peer, id, "files", bytes.NewReader(data), func() bool { return true }); err != nil {
				t.Fatal(err)
			}
			snapshot, err := m.clipboardIncomingSnapshot(peer, "", false)
			if err != nil || snapshot.ID != id {
				t.Fatal("missing internal snapshot", err)
			}
			switch change {
			case "ack":
				_, err = m.SyncClipboard(peer, nil, id)
			case "new-local-copy":
				text := "replacement"
				_, err = m.SyncClipboard(peer, &text, "")
			case "new-remote-copy":
				err = m.clipboardReceivePayload(peer, strings.Repeat("c", 32), "files", bytes.NewReader(testClipboardZip(t, "new.txt", false, "replacement")), func() bool { return true })
			case "selection":
				err = m.SetClipboardPeer(strings.Repeat("d", 64))
			}
			if err != nil || !bytes.Equal(bytes.Clone(snapshot.Data), data) {
				t.Fatal("retirement changed bytes retained for an API copy", err)
			}
		})
	}
}

var clipboardIncomingBenchmarkSink ClipboardValue

func BenchmarkClipboardIncomingSnapshot64MiB(b *testing.B) {
	peer := strings.Repeat("a", 64)
	m := &Manager{clipboard: clipboardState{peer: peer, in: &ClipboardValue{
		ID: strings.Repeat("b", 32), Format: "files", Data: make([]byte, MaxClipboardFileBytes), expires: time.Now().Add(time.Hour),
	}}}
	b.ReportAllocs()
	for b.Loop() {
		clipboardIncomingBenchmarkSink, _ = m.clipboardIncomingSnapshot(peer, "", false)
	}
	clipboardIncomingBenchmarkSink = ClipboardValue{}
}
