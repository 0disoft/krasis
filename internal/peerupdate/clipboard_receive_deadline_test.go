package peerupdate

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type clipboardDeadlineReader func([]byte) (int, error)

func (r clipboardDeadlineReader) Read(data []byte) (int, error) { return r(data) }

type deferredClipboardDeadline struct {
	context.Context
	deadline time.Time
}

func (c deferredClipboardDeadline) Deadline() (time.Time, bool) { return c.deadline, true }

func clipboardDeadlineFixture(t *testing.T, format string) (*Manager, string, string, []byte, time.Time) {
	t.Helper()
	start := time.Now().Add(time.Hour) // Real context timers are never involved.
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	data := []byte("copy")
	if format == "files" {
		data = testClipboardZip(t, "copy.txt", false, "copy")
	}
	if format == "dib" {
		data = make([]byte, 44)
		binary.LittleEndian.PutUint32(data[:4], 40)
		binary.LittleEndian.PutUint32(data[4:8], 1)
		binary.LittleEndian.PutUint32(data[8:12], 1)
		binary.LittleEndian.PutUint16(data[12:14], 1)
		binary.LittleEndian.PutUint16(data[14:16], 32)
	}
	m := &Manager{clipboard: clipboardState{peer: peer, activeUntil: start.Add(5 * time.Second), imageAllowed: true, filesAllowed: true},
		peers: map[string]peerObservation{peer: {info: PeerInfo{Clipboard: true, ClipboardID: id, ClipboardFormat: format, ClipboardSize: len(data)}}}}
	return m, peer, id, data, start
}

func assertClipboardNotPublished(t *testing.T, m *Manager, err error) {
	t.Helper()
	if !errors.Is(err, ErrDenied) || m.clipboard.in != nil || len(m.clipboard.seen) != 0 || m.clipboard.receiving || m.clipboard.transfer.State == "READY" {
		t.Fatalf("failed copy published/retained receive slot: err=%v state=%s receiving=%t seen=%d", err, m.clipboard.transfer.State, m.clipboard.receiving, len(m.clipboard.seen))
	}
}

func TestClipboardReceiveCannotPublishAfterAuthorizationConsumesBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
		m := &Manager{clipboard: clipboardState{peer: peer, activeUntil: time.Now().Add(5 * time.Second)}}
		checks := 0
		err := m.receiveClipboardPayload(peer, id, "", strings.NewReader("copy"), func() bool {
			checks++
			if checks == 2 {
				time.Sleep(12 * time.Second)
			}
			return true // Authority may be read before a scheduling/native I/O stall.
		}, false)
		if !errors.Is(err, ErrDenied) || m.clipboard.in != nil || len(m.clipboard.seen) != 0 {
			t.Fatalf("expired clipboard published: err=%v state=%s seen=%d", err, m.clipboard.transfer.State, len(m.clipboard.seen))
		}
	})
}

func TestClipboardReceiveDeadlineBoundaries(t *testing.T) {
	for _, format := range []string{"", "dib", "files"} {
		for _, phase := range []string{"read", "authorization"} {
			for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
				t.Run(fmt.Sprintf("%s/%s/%s", format, phase, delta), func(t *testing.T) {
					m, peer, id, data, start := clipboardDeadlineFixture(t, format)
					now, checks, reads := start, 0, 0
					advance := func() { now = start.Add(clipboardTransferTimeout(format) + delta) }
					body := clipboardDeadlineReader(func(p []byte) (int, error) {
						reads++
						if phase == "read" {
							advance()
						}
						return copy(p, data), io.EOF
					})
					err := m.receiveClipboardPayload(peer, id, format, body, func() bool {
						checks++
						if phase == "authorization" && checks == 2 {
							advance()
						}
						return true
					}, true, clipboardReceiveOptions{now: func() time.Time { return now }})
					if delta >= 0 {
						assertClipboardNotPublished(t, m, err)
						return
					}
					// GUI activity expired at 5s. An admitted transfer still owns its
					// original 12s/60s budget; it must neither fail early nor renew it.
					if err != nil || m.clipboard.in == nil || m.clipboard.in.ID != id || m.clipboard.transfer.State != "READY" || reads != 1 || m.clipboard.receiving {
						t.Fatal("timely admitted copy rejected", err, reads, m.clipboard.transfer.State)
					}
					if format == "" && m.clipboard.in.Text != string(data) || format != "" && !bytes.Equal(m.clipboard.in.Data, data) {
						t.Fatal("received bytes changed")
					}
				})
			}
		}
	}
}

func TestClipboardReceiveUsesEarlierCallerDeadlineEvenBeforeTimerDelivery(t *testing.T) {
	for _, format := range []string{"", "dib", "files"} {
		for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
			t.Run(fmt.Sprintf("%s/%s", format, delta), func(t *testing.T) {
				m, peer, id, data, start := clipboardDeadlineFixture(t, format)
				now := start
				signal := deferredClipboardDeadline{Context: context.Background(), deadline: start.Add(time.Second)}
				body := clipboardDeadlineReader(func(p []byte) (int, error) { now = signal.deadline.Add(delta); return copy(p, data), io.EOF })
				err := m.receiveClipboardPayload(peer, id, format, body, func() bool { return true }, true, clipboardReceiveOptions{now: func() time.Time { return now }, signal: signal})
				if delta >= 0 {
					assertClipboardNotPublished(t, m, err)
				} else if err != nil || m.clipboard.in == nil {
					t.Fatal("live caller rejected", err)
				}
			})
		}
	}
}

func TestClipboardReceiveAbortStopsReadingAndDoesNotConsumeCopyID(t *testing.T) {
	for _, format := range []string{"", "dib", "files"} {
		for _, before := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/before=%t", format, before), func(t *testing.T) {
				m, peer, id, data, start := clipboardDeadlineFixture(t, format)
				signal, cancel := context.WithCancel(context.Background())
				defer cancel()
				reads, authorizations := 0, 0
				body := clipboardDeadlineReader(func(p []byte) (int, error) { reads++; cancel(); return copy(p, data), nil })
				if before {
					cancel()
				}
				err := m.receiveClipboardPayload(peer, id, format, body, func() bool { authorizations++; return true }, true, clipboardReceiveOptions{now: func() time.Time { return start }, signal: signal})
				assertClipboardNotPublished(t, m, err)
				wantReads := 1
				if before {
					wantReads = 0
				}
				if reads != wantReads || before && authorizations != 0 {
					t.Fatal("aborted receive performed more I/O", reads, authorizations)
				}
				// A failed receive must release its slot and leave the ID retryable.
				if err := m.receiveClipboardPayload(peer, id, format, bytes.NewReader(data), func() bool { return true }, true, clipboardReceiveOptions{now: func() time.Time { return start }}); err != nil || m.clipboard.in == nil {
					t.Fatal("aborted copy poisoned a retry", err)
				}
			})
		}
	}
}

func TestClipboardReceiveExpiredTerminalReceiptDoesNotReadOrRequeue(t *testing.T) {
	for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		t.Run(delta.String(), func(t *testing.T) {
			m, peer, id, _, start := clipboardDeadlineFixture(t, "")
			m.clipboard.seen = []string{id}
			now, checks, reads := start, 0, 0
			body := clipboardDeadlineReader(func([]byte) (int, error) { reads++; return 0, errors.New("terminal body must not be read") })
			err := m.receiveClipboardPayload(peer, id, "", body, func() bool {
				checks++
				if checks == 2 {
					now = start.Add(12*time.Second + delta)
				}
				return true
			}, true, clipboardReceiveOptions{now: func() time.Time { return now }})
			if (err == nil) != (delta < 0) || reads != 0 || m.clipboard.in != nil || len(m.clipboard.seen) != 1 {
				t.Fatal("late receipt accepted or terminal copy reread", err, reads)
			}
		})
	}
}
