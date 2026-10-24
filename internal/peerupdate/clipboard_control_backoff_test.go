package peerupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestClipboardControlBacksOffWithoutUsableRevision(t *testing.T) {
	for _, tc := range []struct {
		name       string
		revision   string
		firstValid bool
	}{
		{"missing", "", false},
		{"invalid", "invalid", false},
		{"overflow", "18446744073709551616", false},
		{"missing-after-valid", "", true},
		{"invalid-after-valid", "invalid", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := testManager(t)
				peer := strings.Repeat("a", 64)
				if err := m.SetClipboardPeer(peer); err != nil {
					t.Fatal(err)
				}
				_, _ = m.SyncClipboard(peer, nil, "")
				info := PeerInfo{Protocol: 2, Generation: clipboardNotifyGeneration}
				m.observePeer(peer, info)
				data, err := json.Marshal(info)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var revisions []string
				client := &http.Client{Transport: clipboardTransportFunc(func(r *http.Request) (*http.Response, error) {
					if r.URL.Path != clipboardStatusPath {
						t.Fatal("unexpected route", r.URL.Path)
					}
					revisions = append(revisions, r.Header.Get("X-Krasis-Clipboard-Revision"))
					header := make(http.Header)
					if tc.firstValid && len(revisions) == 1 {
						header.Set("X-Krasis-Clipboard-Revision", "7")
					} else if tc.revision != "" {
						header.Set("X-Krasis-Clipboard-Revision", tc.revision)
					}
					if len(revisions) == 10 {
						cancel() // Bound the old busy loop as well as the corrected loop.
					}
					return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(bytes.NewReader(data))}, nil
				})}
				start := time.Now()
				m.runClipboardControl(ctx, client, "test.invalid", peer, func() bool { return true })
				intervals := 9
				if tc.firstValid {
					intervals-- // The first usable response can immediately start a wait.
				}
				if elapsed := time.Since(start); elapsed < time.Duration(intervals)*200*time.Millisecond {
					t.Errorf("10 requests took %v; missing revision bypassed retry backoff", elapsed)
				}
				for i, revision := range revisions {
					want := ""
					if tc.firstValid && i == 1 {
						want = "7"
					}
					if revision != want {
						t.Errorf("request %d forwarded revision %q, want %q", i+1, revision, want)
					}
				}
			})
		})
	}
}
