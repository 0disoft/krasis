package inputworker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"testing/iotest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestPreparationWaitReceiptRoundtripAndFollowingFrame(t *testing.T) {
	for _, decision := range []handoff.PreparationDecision{handoff.PreparationUsed, handoff.PreparationNoCandidate, handoff.PreparationExpired} {
		for _, wait := range []time.Duration{0, 132 * time.Millisecond} {
			profile := decisionProfile(decision, true)
			profile.RecordPreparationWait(wait)
			var wire bytes.Buffer
			if err := writeStartupReady(&wire, profile); err != nil || wire.Len() != startupReadyWaitSize || string(wire.Bytes()[:4]) != "KRD6" {
				t.Fatal("service preparation wait envelope missing", err)
			}
			wire.WriteByte('S')
			got, err := readStartupReady(iotest.OneByteReader(&wire))
			if err != nil || got == nil || *got != profile {
				t.Fatal("remaining wait lost or confused with unmeasured", got, err)
			}
			if next, err := wire.ReadByte(); err != nil || next != 'S' {
				t.Fatal("wait envelope consumed following frame")
			}
		}
	}
}

func TestPreparationWaitReceiptTruncationAndInvalidDiagnostics(t *testing.T) {
	profile := decisionProfile(handoff.PreparationExpired, true)
	profile.RecordPreparationWait(time.Millisecond)
	var wire bytes.Buffer
	_ = writeStartupReady(&wire, profile)
	packet := append([]byte(nil), wire.Bytes()...)
	for length := startupReadyDecisionSize; length < startupReadyWaitSize; length++ {
		if _, err := readStartupReady(bytes.NewReader(packet[:length])); !errors.Is(err, ErrProtocol) {
			t.Fatal("truncated wait envelope accepted", length, err)
		}
	}
	for _, offset := range []int{startupReadyDecisionSize + 4, startupReadyDecisionSize + 5, startupReadyDecisionSize + 6, startupReadyDecisionSize + 7} {
		bad := append([]byte(nil), packet...)
		bad[offset] = 2
		if _, err := readStartupReady(bytes.NewReader(bad)); !errors.Is(err, ErrProtocol) {
			t.Fatal("invalid wait framing accepted", offset, err)
		}
	}
	for _, invalidDecision := range []bool{false, true} {
		bad := append([]byte(nil), packet...)
		if invalidDecision {
			bad[startupReadyPreparedSize] = 0
		} else {
			binary.BigEndian.PutUint32(bad[startupReadyDecisionSize:], uint32(handoff.Window.Microseconds())+1)
		}
		reader := bytes.NewReader(append(bad, 'S'))
		if got, err := readStartupReady(reader); err != nil || got != nil {
			t.Fatal("invalid numbers revoked readiness or survived validation", got, err)
		}
		if next, err := reader.ReadByte(); err != nil || next != 'S' {
			t.Fatal("invalid diagnostics left unread wait bytes")
		}
	}
}

func TestWorkerCannotReplaceServicePreparationWait(t *testing.T) {
	service := decisionProfile(handoff.PreparationExpired, false)
	service.RecordPreparationWait(15 * time.Millisecond)
	worker := decisionProfile(handoff.PreparationUsed, true)
	worker.RecordPreparationWait(time.Second)
	mergeWorkerStartup(&service, &worker)
	if !service.Valid() || service.PreparationWaitMicros != 15_000 || !service.PreparationWaitMeasured {
		t.Fatal("worker replaced the service's measured join wait")
	}
}
