package peerupdate

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestClipboardLocalPreparationReleasesSharedLocks(t *testing.T) {
	m := testManager(t)
	m.generation = 72
	peer := strings.Repeat("a", 64)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	data := testClipboardZip(t, "file.txt", false, "original contents")
	want := bytes.Clone(data)
	err := m.queueClipboardLocal(peer, &ClipboardValue{Format: "files", Data: data}, func(local *ClipboardValue) (*ClipboardValue, error) {
		if !m.mu.TryLock() {
			t.Fatal("payload preparation holds the shared connection-manager lock")
		}
		m.mu.Unlock()
		if !m.clipboard.mu.TryLock() {
			t.Fatal("payload preparation holds the clipboard lock")
		}
		m.clipboard.mu.Unlock()
		if m.clipboard.prepareMu.TryLock() {
			m.clipboard.prepareMu.Unlock()
			t.Fatal("large payload preparation is no longer serialized")
		}
		return prepareClipboardLocal(local)
	})
	if err != nil {
		t.Fatal(err)
	}
	data[0] ^= 0xff
	_, id, _ := m.ClipboardInfo(peer)
	snapshot := m.clipboardOutgoing(peer, id)
	if snapshot == nil || !snapshot.legacyFiles {
		t.Fatal("prepared file was not published with legacy compatibility")
	}
	got, err := io.ReadAll(snapshot.body)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("publication did not retain privately owned payload bytes", err)
	}
}

func TestClipboardLocalPreparationCannotPublishSupersededCopy(t *testing.T) {
	for _, action := range []string{"selection-away-and-back", "new-copy", "unshareable-copy", "preparation-failure", "media-disabled"} {
		t.Run(action, func(t *testing.T) {
			m := testManager(t)
			m.generation = 72
			peer := strings.Repeat("a", 64)
			if err := m.SetClipboardPeer(peer); err != nil {
				t.Fatal(err)
			}
			local := &ClipboardValue{Format: "files", Data: testClipboardZip(t, "old.txt", false, "old")}
			err := m.queueClipboardLocal(peer, local, func(local *ClipboardValue) (*ClipboardValue, error) {
				switch action {
				case "selection-away-and-back":
					for _, selected := range []string{strings.Repeat("b", 64), peer} {
						if err := m.SetClipboardPeer(selected); err != nil {
							t.Fatal(err)
						}
					}
				case "new-copy":
					text := "new local copy"
					if _, err := m.SyncClipboard(peer, &text, ""); err != nil {
						t.Fatal(err)
					}
				case "unshareable-copy":
					if _, err := m.SyncClipboardLocalChange(peer, nil, "", true); err != nil {
						t.Fatal(err)
					}
				case "preparation-failure":
					return nil, ErrDenied
				case "media-disabled":
					m.clipboard.mu.Lock()
					m.clipboard.filesAllowed = false
					m.clipboard.mu.Unlock()
				}
				return prepareClipboardLocal(local)
			})
			if !errors.Is(err, ErrDenied) {
				t.Fatal("superseded or failed preparation was published", err)
			}
			m.clipboard.mu.Lock()
			defer m.clipboard.mu.Unlock()
			if action == "new-copy" {
				if m.clipboard.out == nil || m.clipboard.out.Text != "new local copy" {
					t.Fatal("slow old preparation overwrote the newer local copy")
				}
			} else if m.clipboard.out != nil {
				t.Fatal("invalidated preparation restored an old outgoing copy")
			}
		})
	}
}

func TestClipboardLocalPreparationRejectsUnselectedPeerBeforePayloadWork(t *testing.T) {
	m := testManager(t)
	err := m.queueClipboardLocal(strings.Repeat("a", 64), &ClipboardValue{Text: "copy"}, func(*ClipboardValue) (*ClipboardValue, error) {
		t.Fatal("unselected peer reached payload preparation")
		return nil, nil
	})
	if !errors.Is(err, ErrDenied) {
		t.Fatal("unselected peer was accepted", err)
	}
}
