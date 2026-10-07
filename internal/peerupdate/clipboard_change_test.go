package peerupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClipboardChangesBroadcastOnlyOnChanges(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	assertWake := func(change func()) {
		t.Helper()
		revision, wake := m.ClipboardChanges()
		change()
		select {
		case <-wake:
		default:
			t.Fatal("change did not wake subscribers")
		}
		if next, _ := m.ClipboardChanges(); next <= revision {
			t.Fatal("revision did not advance")
		}
	}
	assertWake(func() {
		if err := m.SetClipboardPeer(peer); err != nil {
			t.Fatal(err)
		}
	})
	text := "short copy"
	assertWake(func() {
		if _, err := m.SyncClipboard(peer, &text, ""); err != nil {
			t.Fatal(err)
		}
	})
	_, id, _ := m.ClipboardInfo(peer)
	assertWake(func() { m.clipboardSent(peer, id) })
	info := PeerInfo{Protocol: 2, Generation: clipboardNotifyGeneration, Clipboard: true}
	assertWake(func() { m.observePeer(peer, info) })
	revision, wake := m.ClipboardChanges()
	m.observePeer(peer, info)
	_, _ = m.SyncClipboard(peer, nil, "")
	select {
	case <-wake:
		t.Fatal("unchanged observation or GUI heartbeat generated a wake")
	default:
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := m.WaitClipboardChange(ctx, revision); got != revision {
		t.Fatal("cancel changed revision")
	}
	assertWake(func() {
		if err := m.clipboardReceive(peer, strings.Repeat("b", 32), strings.NewReader("incoming"), func() bool { return true }); err != nil {
			t.Fatal(err)
		}
	})
}

func TestClipboardStatusValidatesRevisionBeforeObservation(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	data, _ := json.Marshal(PeerInfo{Protocol: 2, Generation: clipboardNotifyGeneration, Clipboard: true})
	r := httptest.NewRequest(http.MethodPost, clipboardStatusPath, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Krasis-Clipboard-Revision", "invalid")
	w := httptest.NewRecorder()
	m.handler(peer, func() bool { return true }).ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatal(w.Code)
	}
	if _, ok := m.remoteClipboard(peer); ok {
		t.Fatal("invalid revision mutated observation")
	}
}

// Exercise both TLS roles while a status request is waiting. No management
// ticker is run, so passing proves copies no longer depend on its two-second tick.
func TestClipboardControlWakesBothDirectionsAndLegacy(t *testing.T) {
	for _, generation := range []int{clipboardNotifyGeneration, clipboardNotifyGeneration - 1} {
		for _, senderIsConnector := range []bool{false, true} {
			name := "listener sends"
			if senderIsConnector {
				name = "connector sends"
			}
			if generation < clipboardNotifyGeneration {
				name += " legacy"
			}
			t.Run(name, func(t *testing.T) {
				a, b := testManager(t), testManager(t)
				a.generation, b.generation = clipboardNotifyGeneration, generation
				peer := strings.Repeat("a", 64)
				for _, m := range []*Manager{a, b} {
					if err := m.SetClipboardPeer(peer); err != nil {
						t.Fatal(err)
					}
					_, _ = m.SyncClipboard(peer, nil, "")
				}
				a.observePeer(peer, b.info(peer))
				b.observePeer(peer, a.info(peer))
				var ready atomic.Bool
				var usedNew atomic.Bool
				h := b.handler(peer, func() bool { return true })
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == clipboardStatusPath {
						usedNew.Store(true)
					}
					if r.Header.Get("X-Krasis-Clipboard-Revision") != "" || r.URL.Path == "/v2/status" {
						ready.Store(true)
					}
					h.ServeHTTP(w, r)
				}))
				defer server.Close()
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan struct{})
				go func() {
					defer close(done)
					a.runClipboardControl(ctx, server.Client(), strings.TrimPrefix(server.URL, "https://"), peer, func() bool { return true })
				}()
				defer func() {
					cancel()
					select {
					case <-done:
					case <-time.After(4 * time.Second):
						t.Error("clipboard control did not stop")
					}
				}()
				awaitClipboardCondition(t, ready.Load)
				sender, receiver := b, a
				if senderIsConnector {
					sender, receiver = a, b
				}
				revision, _ := receiver.ClipboardChanges()
				text := "immediate short copy"
				if _, err := sender.SyncClipboard(peer, &text, ""); err != nil {
					t.Fatal(err)
				}
				awaitClipboardCondition(t, func() bool {
					value, err := receiver.SyncClipboard(peer, nil, "")
					return err == nil && value.Text == text
				})
				if got, _ := receiver.ClipboardChanges(); got <= revision {
					t.Fatal("receiver GUI was not notified")
				}
				if usedNew.Load() != (generation >= clipboardNotifyGeneration) {
					t.Fatal("legacy route selection")
				}
			})
		}
	}
}

func awaitClipboardCondition(t *testing.T, ready func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for !ready() {
		select {
		case <-ctx.Done():
			t.Fatal("clipboard notification missed one-second deadline")
		case <-tick.C:
		}
	}
}
