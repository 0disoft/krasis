package inputworker

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/loginagent"
)

func TestLoginAdmissionSeparatesGateWaitAndFreshConsoleRead(t *testing.T) {
	var state atomic.Uint32
	state.Store(uint32(loginagent.UserSession))
	var starts atomic.Int32
	gate := serviceLoginGate(t, &state, &serviceLoginWorker{done: make(chan struct{})}, &starts)
	defer gate.owner.Close()
	var micros atomic.Int64
	clockStarted := make(chan struct{})
	var clockCalls atomic.Int32
	gate.now = func() time.Time {
		value := micros.Load()
		if clockCalls.Add(1) == 1 {
			close(clockStarted)
		}
		return time.UnixMicro(value)
	}
	gate.console = func() (loginagent.Console, error) {
		micros.Store(220_000)
		return loginagent.Console{Session: 1, State: loginagent.UserSession}, nil
	}
	gate.mu.Lock()
	go func() { <-clockStarted; micros.Store(180_000); gate.mu.Unlock() }()
	var profile handoff.ServiceReadinessProfile
	ctx := context.WithValue(context.Background(), loginAdmissionProfileKey{}, &profile)
	if err := gate.admit(ctx); err != nil {
		t.Fatal(err)
	}
	if !profile.Valid() || profile.Micros[handoff.ServiceLoginGateWait] != 180_000 || profile.Micros[handoff.ServiceLoginConsoleRead] != 40_000 {
		t.Fatal("gate contention conflated with fresh WTS queries", profile)
	}
}

func TestExtendedAdmissionReceiptKeepsLegacySizeAndFollowingFrame(t *testing.T) {
	var profile handoff.StartupProfile
	profile.Record(handoff.ServiceLaunch, time.Millisecond)
	profile.ServiceReadiness.Record(handoff.ServiceLoginAdmission, 220*time.Millisecond)
	profile.ServiceReadiness.Record(handoff.ServiceLoginGateWait, 180*time.Millisecond)
	profile.ServiceReadiness.Record(handoff.ServiceLoginConsoleRead, 40*time.Millisecond)
	var wire bytes.Buffer
	if err := writeStartupReady(&wire, profile); err != nil {
		t.Fatal(err)
	}
	packet := append([]byte(nil), wire.Bytes()...)
	if len(packet) != startupReadyAdmissionSize || string(packet[:4]) != "KRDA" {
		t.Fatal("missing bounded admission receipt")
	}
	reader := bytes.NewReader(append(packet, 'S'))
	got, err := readStartupReady(reader)
	if err != nil || got == nil || *got != profile {
		t.Fatal("lost service measurements", got, err)
	}
	if next, err := reader.ReadByte(); err != nil || next != 'S' {
		t.Fatal("consumed following frame")
	}
	for n := 8; n < len(packet); n++ {
		if _, err := readStartupReady(bytes.NewReader(packet[:n])); !errors.Is(err, ErrProtocol) {
			t.Fatal("accepted truncated receipt", n, err)
		}
	}
	for _, offset := range []int{startupReadyImageHashSize, startupReadyImageHashSize + 1, startupReadyImageHashSize + 7} {
		bad := append([]byte(nil), packet...)
		bad[offset] = 4 // Unknown sub-stage or nonzero reserved byte.
		if _, err := readStartupReady(bytes.NewReader(bad)); !errors.Is(err, ErrProtocol) {
			t.Fatal("accepted malformed admission receipt", offset, err)
		}
	}
	if startupReadyServiceSize != startupReadyWaitSize+28 || startupReadyImageHashSize != startupReadyNativeSize+36 {
		t.Fatal("legacy ABI grew")
	}
}
