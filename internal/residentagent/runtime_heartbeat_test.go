package residentagent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/transport"
)

type neverHeartbeatWriter struct{ t *testing.T }

func (w neverHeartbeatWriter) WriteFrameContext(context.Context, transport.Frame) error {
	w.t.Error("inactive pulse wrote a frame")
	return nil
}

func TestProductionInactiveHeartbeatFreezeIncludesCoordinatorAcquisition(t *testing.T) {
	d := newRuntimeDiagnostics()
	runtime, err := application.NewSessionRuntime(agent.RuntimeEffects{}, &testPrivilegedClient{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := peer.NewSessionWriteCoordinator(neverHeartbeatWriter{t})
	if err != nil {
		t.Fatal(err)
	}
	pulse, err := peer.PulseLivenessObserved(context.Background(), c, runtime, time.Now, 2*time.Second, d)
	if err != nil || pulse.Active || pulse.Sent {
		t.Fatal("inactive heartbeat behavior changed", pulse, err)
	}
	d.ObserveFailure(time.Now(), "fault_heartbeat_timeout")
	frozen := <-d.gcFailures
	if p := frozen.heartbeat.Send; p.Measured != 11 || p.Outcome != 2 || p.Micros[0] == 0 || p.Micros[1] < p.Micros[0] || p.Micros[2] != 0 || p.Micros[3] < p.Micros[1] {
		t.Fatal("inactive skip lost gate acquisition or invented write", p)
	}
}

func TestHeartbeatFailureFreezesPendingWriteBeforeCleanup(t *testing.T) {
	d := newRuntimeDiagnostics()
	start := time.Now()
	d.ObserveHeartbeatSend(peer.HeartbeatStart, start)
	d.ObserveHeartbeatSend(peer.HeartbeatGate, start)
	d.ObserveHeartbeatSend(peer.HeartbeatWork, start)
	d.ObserveFailure(start, "fault_heartbeat_timeout")
	frozen := <-d.gcFailures
	// A later cancellation/completion must not rewrite the notification.
	d.ObserveHeartbeatSend(peer.HeartbeatFailed, time.Now())
	d.ObserveHeartbeatSend(peer.HeartbeatComplete, time.Now())
	var ring gcEvidenceRing
	ring.add(productlog.GCWindow{BeginMicros: 1, EndMicros: 2})
	trail := ring.snapshot(frozen, 0)
	if trail.Heartbeat == nil || trail.Heartbeat.AtMicros != trail.TriggerMicros || trail.Heartbeat.Send.Measured != 7 || trail.Heartbeat.Send.Outcome != 0 || trail.Heartbeat.LastSentMicros != 0 {
		t.Fatal("cleanup overwrote pending-write evidence", trail.Heartbeat)
	}
	current := d.heartbeatSnapshot()
	if current.Send.Outcome != 3 || current.Send.Micros[3] == 0 {
		t.Fatal("failed write became success", current)
	}
}

func TestHeartbeatConnectionResetCannotBorrowPriorPeerProgress(t *testing.T) {
	d := newRuntimeDiagnostics()
	at := time.Now()
	for stage := peer.HeartbeatStart; stage <= peer.HeartbeatComplete; stage++ {
		d.ObserveHeartbeatSend(stage, at)
		d.ObserveHeartbeatReceive(stage, at)
	}
	d.ObserveHeartbeatSent("not-exported", at)
	d.ObserveHeartbeatReceived("not-exported", at)
	d.ObserveReadConnectionStarted()
	p := d.heartbeatSnapshot()
	s := d.snapshot(time.Now())
	if p.Send != (productlog.HeartbeatLeg{}) || p.Receive != (productlog.HeartbeatLeg{}) || p.LastSentMicros != 0 || p.LastReceivedMicros != 0 || s.HeartbeatSent || s.HeartbeatReceived {
		t.Fatal("new connection inherited old heartbeat evidence", p, s)
	}
}

func TestNewPendingPulseRetainsOnlyLastSuccessfulWriteBoundary(t *testing.T) {
	var p heartbeatProgressLeg
	for stage := peer.HeartbeatStart; stage <= peer.HeartbeatComplete; stage++ {
		p.observe(stage, int64(stage)+1)
	}
	p.observe(peer.HeartbeatStart, 10)
	p.observe(peer.HeartbeatGate, 12)
	if s := p.snapshot(); s.Measured != 3 || s.Outcome != 0 || s.LastCompleteMicros != 4 {
		t.Fatal("queued pulse erased previous completed write", s)
	}
	p.observe(peer.HeartbeatFailed, 13)
	p.observe(peer.HeartbeatComplete, 14)
	if s := p.snapshot(); s.Outcome != 3 || s.Micros[3] != 13 || s.LastCompleteMicros != 4 {
		t.Fatal("failure or duplicate terminal manufactured progress", s)
	}
}

func TestHeartbeatSnapshotDoesNotPublishMixedOrBusySequence(t *testing.T) {
	var p heartbeatProgressLeg
	p.sequence.Store(1) // Model an interrupted publication, never wait for it.
	p.measured.Store(15)
	if s := p.snapshot(); s != (productlog.HeartbeatLeg{}) {
		t.Fatal("busy publication was guessed", s)
	}
	p.sequence.Store(2)
	p.reset()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := int64(1); i <= 1000; i++ {
			p.observe(peer.HeartbeatStart, i*10)
			p.observe(peer.HeartbeatGate, i*10+1)
			p.observe(peer.HeartbeatWork, i*10+2)
			p.observe(peer.HeartbeatComplete, i*10+3)
		}
	}()
	for i := 0; i < 1000; i++ {
		s := p.snapshot()
		if s.Measured != 0 && (s.Micros[0] < 1 || s.Measured == 15 && (s.Micros[3] != s.Micros[0]+3 || s.Outcome != 1)) {
			t.Fatal("mixed heartbeat attempts", s)
		}
	}
	wg.Wait()
}

func TestHeartbeatProgressCallbacksAndFailureFreezeAllocateNothing(t *testing.T) {
	d := newRuntimeDiagnostics()
	at := time.Now()
	if n := testing.AllocsPerRun(100, func() {
		for stage := peer.HeartbeatStart; stage <= peer.HeartbeatComplete; stage++ {
			d.ObserveHeartbeatSend(stage, at)
			d.ObserveHeartbeatReceive(stage, at)
		}
		d.ObserveFailure(at, "fault_heartbeat_timeout")
		<-d.gcFailures
	}); n != 0 {
		t.Fatal("heartbeat observer allocated", n)
	}
}
