package peerupdate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type countedClipboardBody struct {
	io.Reader
	read *int
}

func (r countedClipboardBody) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	*r.read += n
	return n, err
}

func TestClipboardACKRetryDoesNotDownloadReceivedFilesAgain(t *testing.T) {
	for _, applied := range []bool{false, true} {
		t.Run(fmt.Sprintf("applied=%t", applied), func(t *testing.T) {
			m := testManager(t)
			m.generation = 72
			peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
			data := testClipboardZipBytes(t, "file.bin", bytes.Repeat([]byte("x"), 1<<20))
			if err := m.SetClipboardPeer(peer); err != nil {
				t.Fatal(err)
			}
			_, _ = m.SyncClipboard(peer, nil, "")
			m.observePeer(peer, PeerInfo{Generation: 72, Clipboard: true, ClipboardID: id,
				ClipboardFormat: "files", ClipboardSize: len(data)})
			gets, acks, read := 0, 0, 0
			client := &http.Client{Transport: clipboardTransportFunc(func(r *http.Request) (*http.Response, error) {
				response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}
				if r.Header.Get("X-Krasis-ID") != id {
					t.Error("wrong transfer acknowledged")
				}
				switch r.Method {
				case http.MethodGet:
					gets++
					response.Header.Set("Content-Type", clipboardContentType("files"))
					response.Body = io.NopCloser(countedClipboardBody{Reader: bytes.NewReader(data), read: &read})
				case http.MethodPut:
					acks++
					if acks < 4 {
						response.StatusCode = http.StatusServiceUnavailable
					}
				default:
					t.Error("unexpected method", r.Method)
				}
				return response, nil
			})}
			for attempt := 0; attempt < 4; attempt++ {
				m.pollClipboard(context.Background(), client, "test.invalid", peer, func() bool { return true })
				if applied && attempt == 0 {
					_, _ = m.SyncClipboardPayload(peer, nil, id)
				}
			}
			if gets != 1 || acks != 4 || read != len(data) {
				t.Fatalf("GET=%d ACK=%d payload=%d; want GET=1 ACK=4 payload=%d", gets, acks, read, len(data))
			}
			value, err := m.SyncClipboardPayload(peer, nil, "")
			if err != nil || applied && value.ID != "" || !applied && (value.ID != id || !bytes.Equal(value.Data, data)) {
				t.Fatal("ACK retry changed application state", value.ID, err)
			}
			// A new transfer ID must still fetch its body even with identical bytes.
			id = strings.Repeat("c", 32)
			m.observePeer(peer, PeerInfo{Generation: 72, Clipboard: true, ClipboardID: id,
				ClipboardFormat: "files", ClipboardSize: len(data)})
			m.pollClipboard(context.Background(), client, "test.invalid", peer, func() bool { return true })
			if gets != 2 || acks != 5 || read != 2*len(data) {
				t.Fatal("new copy was treated as an ACK retry", gets, acks, read)
			}
			value, err = m.SyncClipboardPayload(peer, nil, "")
			if err != nil || value.ID != id || !bytes.Equal(value.Data, data) {
				t.Fatal("new copy was not queued", value.ID, err)
			}
		})
	}
}
