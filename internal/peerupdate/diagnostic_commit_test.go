package peerupdate

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestDiagnosticSaveCannotReviveExpiredCollection(t *testing.T) {
	for _, pollStatus := range []bool{false, true} {
		t.Run(map[bool]string{false: "without-status-poll", true: "after-status-expiry"}[pollStatus], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := testManager(t)
				peer, transfer := strings.Repeat("a", 64), strings.Repeat("b", 32)
				request, err := m.QueueDiagnosticRequest(peer, "local.json")
				if err != nil {
					t.Fatal(err)
				}
				m.ReceiveCollection = func(string, string, []byte) (string, error) {
					time.Sleep(time.Minute + time.Nanosecond) // Virtual slow filesystem completion.
					if pollStatus {
						if state, _, _ := m.DiagnosticRequestStatus(peer, request); state != "EXPIRED" {
							t.Fatal("request did not expire", state)
						}
					}
					return "late-peer.json", nil
				}
				err = m.receiveDiagnostic(peer, transfer, request, strings.NewReader(`{}`), func() bool { return true })
				if !errors.Is(err, ErrDenied) {
					t.Error("late save acknowledged", err)
				}
				if state, _, path := m.DiagnosticRequestStatus(peer, request); state != "EXPIRED" || path != "" {
					t.Error("expired collection resurrected", state, path)
				}
				if m.diagnostic.receiving || len(m.diagnostic.last) != 0 || m.diagnostic.received != "" {
					t.Error("late save published a receipt/path or retained admission")
				}
				// Rejection must not prevent a new collection or its exact replay.
				next, err := m.QueueDiagnosticRequest(peer, "next-local.json")
				if err != nil {
					t.Fatal(err)
				}
				saves := 0
				m.ReceiveCollection = func(string, string, []byte) (string, error) { saves++; return "next-peer.json", nil }
				for range 2 {
					if err := m.receiveDiagnostic(peer, transfer, next, strings.NewReader(`{}`), func() bool { return true }); err != nil {
						t.Error("new collection/replay rejected", err)
					}
				}
				if state, _, path := m.DiagnosticRequestStatus(peer, next); state != "RECEIVED" || path != "next-peer.json" || saves != 1 {
					t.Fatal("new collection or replay handling changed", state, path, saves)
				}
			})
		})
	}
}
