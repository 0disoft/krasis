package peerupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestPeerConnectReceiptNeverReplaysAndCloseCancelsJob(t *testing.T) {
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	m := testManager(t)
	var calls atomic.Int32
	started := make(chan struct{})
	m.ConnectPeer = func(ctx context.Context, p string) string {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		return "connection_unconfirmed"
	}
	allowed := func() bool { return true }
	if m.acceptPeerConnect(peer, id, allowed) != "accepted" {
		t.Fatal("not accepted")
	}
	<-started
	for range 3 {
		if m.acceptPeerConnect(peer, id, allowed) != "accepted" {
			t.Fatal("receipt lost")
		}
	}
	if m.acceptPeerConnect(peer, strings.Repeat("c", 32), allowed) != "busy" {
		t.Fatal("queued second job")
	}
	if calls.Load() != 1 {
		t.Fatal("replayed")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if m.acceptPeerConnect(peer, id, func() bool { return false }) != "rejected" {
		t.Fatal("revoked receipt exposed")
	}
	if _, err := m.QueuePeerConnect(peer); err == nil {
		t.Fatal("closed manager queued job")
	}
}

func TestPeerConnectDeadlineIsExactAndLateOrForeignRepliesCannotRenewIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &Manager{}
		peer := strings.Repeat("a", 64)
		id, err := m.QueuePeerConnect(peer)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(peerConnectLifetime - time.Nanosecond)
		if m.PeerConnectStatus(peer, id) != "pending" || m.peerConnectOffer(peer) != id {
			t.Fatal("expired early")
		}
		m.finishPeerConnect(peer, strings.Repeat("b", 32), "connected")
		if m.PeerConnectStatus(peer, id) != "pending" {
			t.Fatal("foreign receipt")
		}
		time.Sleep(time.Nanosecond)
		m.finishPeerConnect(peer, id, "connected")
		if m.PeerConnectStatus(peer, id) != "connection_unconfirmed" || m.peerConnectOffer(peer) != "" {
			t.Fatal("deadline renewed")
		}
	})
}

func TestPeerConnectUsesAuthenticatedControlChannelAndReportsOnlyObservedConnection(t *testing.T) {
	peer := strings.Repeat("a", 64)
	sender, receiver := testManager(t), testManager(t)
	var calls atomic.Int32
	receiver.ConnectPeer = func(context.Context, string) string { calls.Add(1); return "connected" }
	server := httptest.NewTLSServer(sender.handler(peer, func() bool { return true }))
	defer server.Close()
	id, err := sender.QueuePeerConnect(peer)
	if err != nil {
		t.Fatal(err)
	}
	receiver.pollPeerConnect(context.Background(), server.Client(), strings.TrimPrefix(server.URL, "https://"), peer, func() bool { return true })
	receiver.workers.Wait()
	receiver.pollPeerConnect(context.Background(), server.Client(), strings.TrimPrefix(server.URL, "https://"), peer, func() bool { return true })
	if sender.PeerConnectStatus(peer, id) != "connected" || calls.Load() != 1 {
		t.Fatal("wrong receipt or replay")
	}
	request := httptest.NewRequest(http.MethodPost, peerConnectPath, nil)
	request.Header.Set("X-Krasis-Request-ID", id)
	w := httptest.NewRecorder()
	receiver.handler(peer, func() bool { return false }).ServeHTTP(w, request)
	if w.Code != 403 || calls.Load() != 1 {
		t.Fatal("authorization bypass")
	}
	request = httptest.NewRequest(http.MethodPost, peerConnectPath, strings.NewReader(`{"address":"other"}`))
	w = httptest.NewRecorder()
	receiver.handler(peer, func() bool { return true }).ServeHTTP(w, request)
	if w.Code != 400 {
		t.Fatal("accepted caller supplied arguments")
	}
}
