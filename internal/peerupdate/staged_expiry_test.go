package peerupdate

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestExpiredReceiveDoesNotBlockNewBundleWithoutLocalPolling(t *testing.T) {
	for _, state := range []string{"READY", "STAGED"} {
		for _, expired := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", state, expired), func(t *testing.T) {
				m := testManager(t)
				peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
				m.allowed[peer] = true
				m.status = Status{State: state, ID: strings.Repeat("c", 32), Peer: peer}
				m.stagedUntil = time.Now().Add(time.Minute)
				if expired {
					m.stagedUntil = time.Now().Add(-time.Minute)
				}
				bundle := &fakeBundle{generation: 22}
				m.validate = func(io.ReaderAt, int64) (stagedBundle, error) { return bundle, nil }
				data := []byte("verified fixture")
				if !expired {
					if err := m.receive(peer, id, fmt.Sprintf("%x", sha256.Sum256(data)), panicReader{}, func() bool { return true }); err != ErrBusy {
						t.Fatal("live job overwritten", err)
					}
					return
				}
				if err := m.receive(peer, id, fmt.Sprintf("%x", sha256.Sum256(data)), bytes.NewReader(data), func() bool { return true }); err != nil || !bundle.staged || m.status.ID != id || m.status.State != "STAGED" {
					t.Fatal("expired job blocked fresh reception", err, m.status)
				}
			})
		}
	}
}

func TestIncomingExpiryNeverClearsActiveClaimAndIsObservableOnce(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	m.allowed[peer] = true
	m.status = Status{State: "APPLYING", ID: strings.Repeat("b", 32), Peer: peer}
	m.stagedUntil = time.Now().Add(-time.Minute)
	if err := m.receive(peer, strings.Repeat("c", 32), strings.Repeat("d", 64), panicReader{}, func() bool { return true }); err != ErrBusy || m.info(peer).UpdateState != "" || m.status.State != "APPLYING" {
		t.Fatal("active claim expired", err)
	}
	events := 0
	m.Observe = func(code string) {
		if code == "peer_update_receive_expired" {
			events++
		}
	}
	m.status.State = "READY"
	m.info(peer)
	m.info(peer)
	if m.status.State != "EXPIRED" || events != 1 {
		t.Fatal("remote observations did not expire exactly once", m.status, events)
	}
}
