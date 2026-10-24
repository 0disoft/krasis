package peerupdate

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiagnosticHandlerCannotOccupyBundleGate(t *testing.T) {
	for _, path := range []string{"/v1/diagnostic-transfer", "/v1/diagnostic-request"} {
		t.Run(path, func(t *testing.T) {
			m := testManager(t)
			peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
			archive := queuedFileOffer(t, m, peer)
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			finish := func() { releaseOnce.Do(func() { close(release) }) }
			defer finish()
			var calls atomic.Int32
			block := func() {
				if calls.Add(1) == 1 {
					close(entered)
				}
				<-release
			}
			m.ReceiveDiagnostic = func(string, []byte) (string, error) { block(); return "diagnostic.json", nil }
			m.ExportDiagnostic = func(string, string) error { block(); return nil }
			request := httptest.NewRequest(http.MethodPost, path, nil)
			if path == "/v1/diagnostic-transfer" {
				request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("X-Krasis-ID", id)
			} else {
				request.Header.Set("X-Krasis-Request-ID", id)
			}
			handler := m.handler(peer, func() bool { return true })
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				handler.ServeHTTP(response, request)
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("diagnostic operation did not start")
			}
			duplicate := httptest.NewRecorder()
			handler.ServeHTTP(duplicate, httptest.NewRequest(http.MethodGet, "/v1/diagnostic-transfer", nil))
			if duplicate.Code != http.StatusTooManyRequests || calls.Load() != 1 {
				t.Fatal("diagnostic concurrency was not bounded", duplicate.Code, calls.Load())
			}
			bundle := httptest.NewRecorder()
			handler.ServeHTTP(bundle, httptest.NewRequest(http.MethodGet, "/v1/bundle", nil))
			if bundle.Code != http.StatusOK || int64(bundle.Body.Len()) != archive.size || bundle.Header().Get("X-Krasis-ID") != archive.id {
				t.Errorf("active diagnostic blocked queued bundle: HTTP %d, bytes=%d", bundle.Code, bundle.Body.Len())
			}
			finish()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("diagnostic handler did not stop")
			}
			if response.Code != http.StatusOK {
				t.Fatal("diagnostic failed", response.Code)
			}
			after := httptest.NewRecorder()
			handler.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/v1/diagnostic-transfer", nil))
			if after.Code != http.StatusNoContent {
				t.Fatal("diagnostic gate was not released", after.Code)
			}
		})
	}
}
