package peerupdate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiagnosticTransferRetriesAfterHandlerAdmissionThrottle(t *testing.T) {
	sender, receiver := testManager(t), testManager(t)
	senderID, receiverID := strings.Repeat("a", 64), strings.Repeat("b", 64)
	requestID, err := receiver.QueueDiagnosticRequest(senderID, "local.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = sender.QueueDiagnosticForRequest(receiverID, requestID, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	offer := sender.diagnosticOffer(receiverID)
	var saves atomic.Int32
	receiver.ReceiveCollection = func(peer, id string, data []byte) (string, error) {
		if peer != senderID || id != requestID || string(data) != `{}` {
			return "", fmt.Errorf("diagnostic identity or content changed")
		}
		saves.Add(1)
		return "peer.json", nil
	}
	started, release := make(chan struct{}), make(chan struct{})
	receiver.ExportDiagnosticContext = func(ctx context.Context, _, _ string) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	server := httptest.NewTLSServer(receiver.handler(senderID, func() bool { return true }))
	defer server.Close()
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := server.Client()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/diagnostic-request", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Krasis-Request-ID", strings.Repeat("c", 32))
	done := make(chan error, 1)
	go func() {
		resp, err := client.Do(req)
		if resp != nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("blocking diagnostic request: HTTP %d", resp.StatusCode)
			}
		}
		done <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("diagnostic request did not acquire the handler slot")
	}
	address := strings.TrimPrefix(server.URL, "https://")
	sender.pollDiagnostic(ctx, client, address, receiverID, func() bool { return true })
	current := sender.diagnosticOffer(receiverID)
	if current == nil || current.id != offer.id || !current.expires.Equal(offer.expires) || !sender.DiagnosticRequestHandled(receiverID, requestID) || saves.Load() != 0 {
		t.Fatal("handler throttle discarded the pending transfer or its request marker", sender.Status().Diagnostic)
	}
	unblock()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("diagnostic request did not release the handler slot")
	}
	sender.pollDiagnostic(ctx, client, address, receiverID, func() bool { return true })
	state, _, path := receiver.DiagnosticRequestStatus(senderID, requestID)
	if state != "RECEIVED" || path != "peer.json" || sender.Status().Diagnostic != "SENT" || saves.Load() != 1 || !sender.DiagnosticRequestHandled(receiverID, requestID) {
		t.Fatal("diagnostic transfer did not recover after handler throttle", state, path, saves.Load(), sender.Status().Diagnostic)
	}
}
