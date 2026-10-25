package peerupdate

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestDiagnosticPollDoesNotPublishAfterCancellation(t *testing.T) {
	for _, stage := range []string{"body-cancel", "save-cancel", "save-deadline"} {
		t.Run(stage, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := testManager(t)
				peer, transfer := strings.Repeat("a", 64), strings.Repeat("b", 32)
				request, err := m.QueueDiagnosticRequest(peer, "local.json")
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				first, saves, acknowledgements := true, 0, 0
				m.ReceiveCollection = func(string, string, []byte) (string, error) {
					saves++
					if first {
						if stage == "save-cancel" {
							cancel()
						}
						if stage == "save-deadline" {
							time.Sleep(diagnosticPollTimeout + time.Nanosecond)
						}
					}
					return "peer.json", nil
				}
				client := &http.Client{Transport: clipboardTransportFunc(func(r *http.Request) (*http.Response, error) {
					if err := r.Context().Err(); err != nil {
						return nil, err
					}
					if r.Method == http.MethodPut {
						acknowledgements++
						return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
					}
					body := strings.NewReader(`{}`)
					reader := diagnosticAdmissionReader{read: func(p []byte) (int, error) {
						n, err := body.Read(p)
						if first && stage == "body-cancel" && err == io.EOF {
							cancel()
						}
						return n, err
					}}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(reader), Header: http.Header{"X-Krasis-Id": {transfer}, "X-Krasis-Request-Id": {request}}}, nil
				})}
				m.pollDiagnostic(ctx, client, "127.0.0.1:1", peer, func() bool { return true })
				wantSaves := 1
				if stage == "body-cancel" {
					wantSaves = 0
				}
				if saves != wantSaves {
					t.Errorf("canceled download invoked storage: got=%d want=%d", saves, wantSaves)
				}
				if state, _, path := m.DiagnosticRequestStatus(peer, request); state != "PENDING" || path != "" {
					t.Error("canceled poll published collection success", state, path)
				}
				if m.diagnostic.receiving || len(m.diagnostic.last) != 0 || m.diagnostic.received != "" || acknowledgements != 0 {
					t.Error("canceled poll retained admission or published a receipt/path/ACK")
				}
				// The same live collection remains retryable, with exact replay deduplication.
				first = false
				beforeRetry := saves
				for range 2 {
					m.pollDiagnostic(context.Background(), client, "127.0.0.1:1", peer, func() bool { return true })
				}
				if state, _, path := m.DiagnosticRequestStatus(peer, request); state != "RECEIVED" || path != "peer.json" || saves != beforeRetry+1 || acknowledgements != 2 {
					t.Error("live retry or replay changed", state, path, saves, acknowledgements)
				}
			})
		})
	}
}
