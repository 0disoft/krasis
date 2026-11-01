package peerupdate

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

type bundleFlushResponse struct {
	*httptest.ResponseRecorder
	flush func() error
}

func (w bundleFlushResponse) FlushError() error { return w.flush() }

func TestReceiveBundleRequiresFlushAndLiveTrust(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(fmt.Sprintf("revoke-on-flush=%t", revoke), func(t *testing.T) {
			m := testManager(t)
			peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
			if err := m.Permit(peer, true); err != nil {
				t.Fatal(err)
			}
			m.validate = func(io.ReaderAt, int64) (stagedBundle, error) { return &fakeBundle{generation: 22}, nil }
			data := []byte("bounded fixture")
			request := httptest.NewRequest(http.MethodPost, "/v1/bundle", bytes.NewReader(data))
			request.Header.Set("Content-Type", "application/zip")
			request.Header.Set("X-Krasis-ID", id)
			request.Header.Set("X-Krasis-SHA256", fmt.Sprintf("%x", sha256.Sum256(data)))
			allowed, flushed := true, false
			response := bundleFlushResponse{httptest.NewRecorder(), func() error {
				flushed = true
				if revoke {
					allowed = false
					return nil
				}
				return errors.New("response flush failed")
			}}
			m.handler(peer, func() bool { return allowed }).ServeHTTP(response, request)
			if response.Code != http.StatusOK || !flushed || m.Status().State != "STAGED" {
				t.Fatalf("unsafe readiness: status=%d flushed=%t state=%s", response.Code, flushed, m.Status().State)
			}
		})
	}
}

func TestAuxiliaryHandlerPreservesAdmissionPriorityAndLaneIsolation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := testManager(t)
		var allowed atomic.Bool
		allowed.Store(true)
		handler := m.handler(strings.Repeat("a", 64), allowed.Load)
		reader, writer := io.Pipe()
		defer reader.Close()
		defer writer.Close()
		request := httptest.NewRequest(http.MethodPost, "/v2/status", reader)
		request.Header.Set("Content-Type", "application/json")
		done := make(chan struct{})
		go func() {
			defer close(done)
			handler.ServeHTTP(httptest.NewRecorder(), request)
		}()
		synctest.Wait() // Body read holds the control lane, without holding m.mu.

		allowed.Store(false)
		denied := httptest.NewRecorder()
		handler.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/v2/status?invalid=1", panicReader{}))
		if denied.Code != http.StatusForbidden {
			t.Fatalf("trust must precede lane/query/body checks: HTTP %d", denied.Code)
		}
		allowed.Store(true)
		for _, path := range []string{"/v2/status", "/v1/app-open", peerConnectPath} {
			busy := httptest.NewRecorder()
			handler.ServeHTTP(busy, httptest.NewRequest(http.MethodPost, path+"?invalid=1", panicReader{}))
			if busy.Code != http.StatusTooManyRequests {
				t.Fatalf("%s must share the control lane before query/body checks: HTTP %d", path, busy.Code)
			}
		}
		bundle := httptest.NewRecorder()
		handler.ServeHTTP(bundle, httptest.NewRequest(http.MethodGet, "/v1/bundle", nil))
		if bundle.Code != http.StatusNoContent {
			t.Fatalf("control request blocked independent bundle lane: HTTP %d", bundle.Code)
		}
		_ = writer.Close()
		<-done
		after := httptest.NewRecorder()
		handler.ServeHTTP(after, httptest.NewRequest(http.MethodPost, "/v2/status?invalid=1", panicReader{}))
		if after.Code != http.StatusBadRequest || after.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("lane was not released or query check changed: HTTP %d", after.Code)
		}
	})
}

func TestBundleReceiveHTTPStatusPreservesErrorPriority(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, http.StatusOK},
		{"denied", ErrDenied, http.StatusForbidden},
		{"wrapped denied", fmt.Errorf("receive: %w", ErrDenied), http.StatusForbidden},
		{"busy", ErrBusy, http.StatusConflict},
		{"wrapped busy", fmt.Errorf("receive: %w", ErrBusy), http.StatusConflict},
		{"denied takes precedence", errors.Join(ErrBusy, ErrDenied), http.StatusForbidden},
		{"other failure", errors.New("storage failure"), http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := bundleReceiveHTTPStatus(tc.err); got != tc.want {
				t.Fatalf("status=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestAuxiliaryHandlerPreservesMethodAndInputErrors(t *testing.T) {
	m := testManager(t)
	handler := m.handler(strings.Repeat("a", 64), func() bool { return true })
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/v2/status", http.StatusNotFound},
		{http.MethodDelete, "/v1/bundle", http.StatusNotFound},
		{http.MethodGet, "/v1/receipt", http.StatusNotFound},
		{http.MethodPost, "/v2/status", http.StatusBadRequest},
		{http.MethodPost, "/v1/bundle", http.StatusBadRequest},
		{http.MethodPost, "/v1/receipt", http.StatusBadRequest},
		{http.MethodGet, "/unknown", http.StatusNotFound},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, panicReader{})
			request.Header.Set("X-Krasis-Result", "invalid")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status=%d want=%d", response.Code, tc.want)
			}
			if request.URL.Path != tc.path || request.Header.Get("X-Krasis-Result") != "invalid" {
				t.Fatal("handler mutated caller-owned routing or receipt metadata")
			}
		})
	}
}
