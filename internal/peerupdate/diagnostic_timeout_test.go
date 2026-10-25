package peerupdate

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestDiagnosticUploadTimeoutReleasesReceiveSlot(t *testing.T) {
	for _, shorterParent := range []bool{false, true} {
		t.Run(map[bool]string{false: "own-budget", true: "shorter-parent"}[shorterParent], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := testManager(t)
				peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
				saves := 0
				m.ReceiveDiagnostic = func(string, []byte) (string, error) { saves++; return "saved.json", nil }
				ctx := context.Background()
				budget := diagnosticPollTimeout
				if shorterParent {
					budget = time.Second
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, budget)
					defer cancel()
				}
				writer := &delayedClipboardInterrupt{
					header: make(http.Header), readStarted: make(chan struct{}), readInterrupted: make(chan struct{}),
					finishDeadline: make(chan struct{}), status: make(chan int, 1),
				}
				close(writer.finishDeadline)
				r := httptest.NewRequest(http.MethodPost, "/v1/diagnostic-transfer", io.NopCloser(writer)).WithContext(ctx)
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-Krasis-ID", id)
				h := m.handler(peer, func() bool { return true })
				done := make(chan struct{})
				go func() { defer close(done); h.ServeHTTP(writer, r) }()
				<-writer.readStarted
				time.Sleep(budget - time.Nanosecond)
				synctest.Wait()
				select {
				case <-done:
					t.Error("upload interrupted before its deadline")
				default:
				}
				time.Sleep(time.Nanosecond)
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Error("diagnostic upload retained its receive slot beyond the deadline")
					_ = writer.SetReadDeadline(time.Now())
					<-done // Join the blocked fixture even against the old implementation.
				}
				if status := <-writer.status; status != http.StatusUnprocessableEntity || saves != 0 {
					t.Fatal("timed-out diagnostic was saved or acknowledged", status, saves)
				}
				next := httptest.NewRequest(http.MethodPost, "/v1/diagnostic-transfer", strings.NewReader(`{}`))
				next.Header.Set("Content-Type", "application/json")
				next.Header.Set("X-Krasis-ID", strings.Repeat("c", 32))
				response := httptest.NewRecorder()
				h.ServeHTTP(response, next)
				if response.Code != http.StatusOK || saves != 1 {
					t.Fatal("timed-out upload leaked its handler/receive slot", response.Code, saves)
				}
			})
		})
	}
}

func TestDiagnosticUploadJoinsReadInterrupt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := testManager(t)
		m.ReceiveDiagnostic = func(string, []byte) (string, error) {
			t.Error("cancelled diagnostic reached storage")
			return "saved.json", nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		writer := &delayedClipboardInterrupt{
			header: make(http.Header), readStarted: make(chan struct{}), readInterrupted: make(chan struct{}),
			finishDeadline: make(chan struct{}), status: make(chan int, 1),
		}
		r := httptest.NewRequest(http.MethodPost, "/v1/diagnostic-transfer", io.NopCloser(writer)).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Krasis-ID", strings.Repeat("b", 32))
		done := make(chan struct{})
		go func() {
			defer close(done)
			m.handler(strings.Repeat("a", 64), func() bool { return true }).ServeHTTP(writer, r)
		}()
		<-writer.readStarted
		cancel()
		if status := <-writer.status; status != http.StatusUnprocessableEntity {
			t.Error("cancelled diagnostic was accepted", status)
		}
		synctest.Wait()
		select {
		case <-done:
			t.Error("handler returned before its deadline callback finished")
		default:
		}
		close(writer.finishDeadline)
		<-done
	})
}
