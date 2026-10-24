package peerupdate

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type diagnosticAdmissionReader struct {
	read func([]byte) (int, error)
}

func (r diagnosticAdmissionReader) Read(p []byte) (int, error) { return r.read(p) }

func TestDiagnosticReceiveReservesSlotBeforeReadingBody(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	m.ReceiveDiagnostic = func(string, []byte) (string, error) { return "peer.json", nil }
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	body := strings.NewReader(`{}`)
	first := true
	reader := diagnosticAdmissionReader{read: func(p []byte) (int, error) {
		if first {
			first = false
			close(started)
			<-release
		}
		return body.Read(p)
	}}
	go func() {
		done <- m.receiveDiagnostic(peer, strings.Repeat("b", 32), "", reader, func() bool { return true })
	}()
	t.Cleanup(func() {
		close(release)
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(time.Second):
			t.Error("diagnostic receiver did not finish")
		}
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("diagnostic body was not read")
	}
	reads := 0
	second := diagnosticAdmissionReader{read: func([]byte) (int, error) {
		reads++
		return 0, io.EOF
	}}
	err := m.receiveDiagnostic(peer, strings.Repeat("c", 32), "", second, func() bool { return true })
	if !errors.Is(err, ErrBusy) || reads != 0 {
		t.Fatalf("busy receiver consumed another body: reads=%d err=%v", reads, err)
	}
	if _, err = m.QueueDiagnosticRequest(peer, "new.json"); !errors.Is(err, ErrBusy) {
		t.Fatal("request replaced during body read", err)
	}
	m.Status()
}

func TestDiagnosticReadFailureReleasesReceiveSlot(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	m.ReceiveDiagnostic = func(string, []byte) (string, error) { return "peer.json", nil }
	reader := diagnosticAdmissionReader{read: func([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }}
	if err := m.receiveDiagnostic(peer, strings.Repeat("b", 32), "", reader, func() bool { return true }); !errors.Is(err, ErrDenied) {
		t.Fatal("body read failure accepted", err)
	}
	if err := m.receiveDiagnostic(peer, strings.Repeat("c", 32), "", strings.NewReader(`{}`), func() bool { return true }); err != nil {
		t.Fatal("failed body read leaked receive slot", err)
	}
}

func TestDiagnosticReceiveDoesNotRaceLocalSnapshotPreparation(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	m.ReceiveDiagnostic = func(string, []byte) (string, error) { return "peer.json", nil }
	_, err := m.PrepareDiagnosticRequest(peer, func(string) (string, error) {
		reads := 0
		reader := diagnosticAdmissionReader{read: func([]byte) (int, error) {
			reads++
			return 0, io.EOF
		}}
		err := m.receiveDiagnostic(peer, strings.Repeat("b", 32), "", reader, func() bool { return true })
		if !errors.Is(err, ErrBusy) || reads != 0 {
			t.Fatal("receiver entered during local snapshot preparation", reads, err)
		}
		return "local.json", nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
