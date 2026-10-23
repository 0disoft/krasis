package peerupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
)

func TestCloseKeepsRootUntilHTTPHandlerReturns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := testManager(t)
		reader, writer := io.Pipe()
		defer reader.Close()
		defer writer.Close()
		var authorizations atomic.Int32
		handler := m.handler(strings.Repeat("a", 64), func() bool {
			authorizations.Add(1)
			return true
		})
		request := httptest.NewRequest(http.MethodPost, "/v2/status", reader)
		request.Header.Set("Content-Type", "application/json")
		done := make(chan struct{})
		go func() {
			defer close(done)
			handler.ServeHTTP(httptest.NewRecorder(), request)
		}()
		synctest.Wait() // The admitted request is blocked reading its body.
		if authorizations.Load() != 1 {
			t.Fatal("request was not admitted")
		}
		expired, cancel := context.WithCancel(context.Background())
		cancel()
		if err := m.closeContext(expired); !errors.Is(err, ErrBusy) {
			t.Errorf("Close returned %v while a handler was still active", err)
		}
		synctest.Wait()
		select {
		case <-m.closeDone:
			t.Error("manager cleanup completed before its handler")
		default:
		}
		if _, err := m.root.Stat("."); err != nil {
			t.Errorf("active handler lost its root: %v", err)
		}
		late := httptest.NewRecorder()
		handler.ServeHTTP(late, httptest.NewRequest(http.MethodGet, "/v2/status", nil))
		if late.Code != http.StatusServiceUnavailable || authorizations.Load() != 1 {
			t.Errorf("late request reached closed manager: status=%d authorization calls=%d", late.Code, authorizations.Load())
		}
		_ = writer.Close()
		<-done
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := m.root.Stat("."); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("root was not closed after the last handler: %v", err)
		}
	})
}
