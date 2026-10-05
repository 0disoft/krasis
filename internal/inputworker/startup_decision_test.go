package inputworker

import (
	"bytes"
	"testing"
	"testing/iotest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func decisionProfile(decision handoff.PreparationDecision, load bool) handoff.StartupProfile {
	var p handoff.StartupProfile
	p.Record(handoff.ServiceLaunch, time.Millisecond)
	p.Record(handoff.ServiceChildConnect, 2*time.Millisecond)
	p.PreparationDecision = decision
	if decision == handoff.PreparationUsed {
		p.Prepared, p.PreparedAgeMicros = true, 1000
	}
	if load {
		p.Record(handoff.WorkerInstalledValidation, 3*time.Millisecond)
		p.RecordLoad(time.Millisecond)
	}
	return p
}

func TestDecisionReadyRoundtripAndFollowingFrame(t *testing.T) {
	for decision := handoff.PreparationUsed; decision <= handoff.PreparationInvalidAge; decision++ {
		for _, load := range []bool{false, true} {
			p := decisionProfile(decision, load)
			var wire bytes.Buffer
			if err := writeStartupReady(&wire, p); err != nil || wire.Len() != startupReadyDecisionSize || string(wire.Bytes()[:4]) != "KRD5" {
				t.Fatal("fixed decision receipt missing", err)
			}
			wire.WriteByte('S')
			got, err := readStartupReady(iotest.OneByteReader(&wire))
			if err != nil || got == nil || *got != p {
				t.Fatal("service preparation decision lost", got, err)
			}
			if next, err := wire.ReadByte(); err != nil || next != 'S' {
				t.Fatal("decision receipt consumed the following frame")
			}
		}
	}
}

func TestDecisionReadyFramingFailsClosedButInvalidDiagnosticDoesNotRevokeReadiness(t *testing.T) {
	p := decisionProfile(handoff.PreparationExpired, true)
	var wire bytes.Buffer
	_ = writeStartupReady(&wire, p)
	packet := append([]byte(nil), wire.Bytes()...)
	for length := startupReadyPreparedSize; length < startupReadyDecisionSize; length++ {
		if _, err := readStartupReady(bytes.NewReader(packet[:length])); err != ErrProtocol {
			t.Fatal("truncated decision receipt accepted", length)
		}
	}
	for i := startupReadyPreparedSize + 1; i < startupReadyDecisionSize; i++ {
		bad := append([]byte(nil), packet...)
		bad[i] = 1
		if _, err := readStartupReady(bytes.NewReader(bad)); err != ErrProtocol {
			t.Fatal("nonzero reserved decision byte accepted")
		}
	}
	for _, decision := range []byte{0, byte(handoff.PreparationUsed), 255} {
		bad := append([]byte(nil), packet...)
		bad[startupReadyPreparedSize] = decision
		reader := bytes.NewReader(append(bad, 'S'))
		if got, err := readStartupReady(reader); err != nil || got != nil {
			t.Fatal("invalid diagnostic changed readiness", got, err)
		}
		if next, err := reader.ReadByte(); err != nil || next != 'S' {
			t.Fatal("discarded decision misaligned receipt framing")
		}
	}
}

func TestWorkerCannotReplaceServicePreparationDecision(t *testing.T) {
	service := decisionProfile(handoff.PreparationExpired, false)
	worker := decisionProfile(handoff.PreparationUsed, true)
	mergeWorkerStartup(&service, &worker)
	if !service.Valid() || service.Prepared || service.PreparedAgeMicros != 0 || service.PreparationDecision != handoff.PreparationExpired || !service.LoadMeasured {
		t.Fatal("child replaced the service's candidate selection")
	}
}
