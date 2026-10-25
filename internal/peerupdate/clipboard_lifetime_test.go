package peerupdate

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type clipboardTransportFunc func(*http.Request) (*http.Response, error)

func (f clipboardTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewTextCancelsBlockedFileAndIsSentBeforeExpiry(t *testing.T) {
	m := testManager(t)
	m.generation = 257
	peer := strings.Repeat("a", 64)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	m.observePeer(peer, PeerInfo{Generation: 257, Clipboard: true})
	files := testClipboardZip(t, "file.txt", false, "file F")
	_, _ = m.SyncClipboardPayload(peer, &ClipboardValue{Format: "files", Data: files}, "")
	started, cancelled := make(chan struct{}), make(chan struct{})
	var delivered string
	c := &http.Client{Transport: clipboardTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Content-Type") == clipboardContentType("files") {
			close(started)
			<-r.Context().Done()
			close(cancelled)
			return nil, r.Context().Err()
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		delivered = string(data)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { m.pollClipboard(ctx, c, "test.invalid", peer, func() bool { return true }); close(done) }()
	<-started
	text := "text T"
	if _, err := m.SyncClipboard(peer, &text, ""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("file did not cancel")
	}
	<-done
	m.pollClipboard(ctx, c, "test.invalid", peer, func() bool { return true })
	if delivered != text {
		t.Fatal("new text not sent", delivered)
	}
}

func TestTransferIgnoresUnrelatedWakeButCancelsSelectionOrRemoteIDChange(t *testing.T) {
	for _, remoteChange := range []bool{false, true} {
		m := testManager(t)
		peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
		if err := m.SetClipboardPeer(peer); err != nil {
			t.Fatal(err)
		}
		_, _ = m.SyncClipboard(peer, nil, "")
		m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: id, ClipboardSize: 1})
		ctx, stop, current := m.clipboardTransferContext(context.Background(), peer, id, "", 1, false)
		m.clipboardChanged()
		if !current() || ctx.Err() != nil {
			t.Fatal("unchanged copy cancelled by wake")
		}
		if remoteChange {
			m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: strings.Repeat("c", 32), ClipboardSize: 1})
		} else {
			_, _ = m.ClearClipboardPeer(peer)
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("stale transfer survived")
		}
		stop()
	}
}

func TestReceiveRechecksAdvertisedIDBeforePublishing(t *testing.T) {
	m := testManager(t)
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SyncClipboard(peer, nil, "")
	m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: id, ClipboardSize: 3})
	reader, writer := io.Pipe()
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		done <- m.receiveClipboardPayload(peer, id, "", &signalClipboardReader{Reader: reader, started: started}, func() bool { return true }, true)
	}()
	<-started
	m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: strings.Repeat("c", 32), ClipboardSize: 3})
	_, _ = writer.Write([]byte("old"))
	writer.Close()
	if err := <-done; err == nil {
		t.Fatal("stale advertised copy accepted")
	}
	if value, _ := m.SyncClipboard(peer, nil, ""); value.ID != "" {
		t.Fatal(value)
	}
}

func TestIgnoredAdvertisedCopyCannotBeFetchedAgainAfterLocalChange(t *testing.T) {
	m := testManager(t)
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	if err := m.SetClipboardPeer(peer); err != nil {
		t.Fatal(err)
	}
	_, _ = m.SyncClipboard(peer, nil, "")
	m.observePeer(peer, PeerInfo{Clipboard: true, ClipboardID: id, ClipboardSize: 3})
	_, _ = m.SyncClipboardLocalChange(context.Background(), peer, nil, "", true)
	if err := m.receiveClipboardPayload(peer, id, "", strings.NewReader("old"), func() bool { return true }, true); err != nil {
		t.Fatal(err)
	}
	if value, _ := m.SyncClipboard(peer, nil, ""); value.ID != "" {
		t.Fatal("ignored remote copy queued again", value)
	}
}
