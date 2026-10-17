package peerupdate

import (
	"net/http"
	"net/http/httptest"
	"os"
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
	m := testManager(t)
	m.OpenUI = func(string) string { return "launch_requested" }
	name := archiveNameFor(id)
	file, err := m.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("bulk")); err != nil {
		t.Fatal(err)
	}
	m.out = &offer{peer: peer, id: id, file: file, name: name, size: 4, expires: time.Now().Add(time.Minute)}
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
