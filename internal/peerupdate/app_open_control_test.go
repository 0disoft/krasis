package peerupdate

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type stalledTransferWriter struct {
	*httptest.ResponseRecorder
	started, release chan struct{}
}

func (w *stalledTransferWriter) Write(p []byte) (int, error) {
	close(w.started)
	<-w.release
	return w.ResponseRecorder.Write(p)
}

func TestAppOpenControlDoesNotWaitForBulkResponse(t *testing.T) {
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	m := &Manager{OpenUI: func(string) string { return "launch_requested" }, out: &offer{peer: peer, id: id, data: []byte("bulk"), expires: time.Now().Add(time.Minute)}}
	allowed := true
	h := m.handler(peer, func() bool { return allowed })
	w := &stalledTransferWriter{httptest.NewRecorder(), make(chan struct{}), make(chan struct{})}
	done := make(chan struct{})
	go func() { defer close(done); h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/bundle", nil)) }()
	defer func() { close(w.release); <-done }()
	<-w.started
	request := httptest.NewRequest(http.MethodPost, "/v1/app-open", nil)
	request.Header.Set("X-Krasis-Request-ID", id)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 200 || response.Header().Get("X-Krasis-Result") != "launch_accepted" {
		t.Fatal("bulk transfer blocked app launch", response.Code)
	}
	awaitAppOpenReceipt(t, m, peer, id, func() bool { return allowed })
	allowed = false
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("independent control bypassed trust")
	}
}
