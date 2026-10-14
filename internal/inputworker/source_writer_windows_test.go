//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/wininput"
)

func TestSourceWriterCannotPreemptServiceReadinessWithEvents(t *testing.T) {
	w := &sourceWriter{}
	for _, kind := range []byte{sourceBoundary, sourceAction, sourceReturn} {
		if err := w.send(sourcePacket{kind: kind}, nil); err != nil || w.sequence != 0 {
			t.Fatal("event reached an unstarted stream", err)
		}
	}
}

func TestSourceSenderAuthorityLossJoinsBeforeCleanRetirement(t *testing.T) {
	agent, conn := net.Pipe()
	_ = agent.Close()
	defer conn.Close()
	writer := &sourceWriter{channel: &Channel{stream: conn}, binding: Binding{1}}
	writer.started.Store(true)
	session, err := wininput.NewSourceSession(4, writer)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- session.RunWithReady(ctx, func() { close(ready) }) }()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("source sender did not start")
	}
	if session.Activate() != nil || !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
		t.Fatal("source session did not accept synthetic queue item")
	}
	var sender error
	select {
	case sender = <-done:
	case <-ctx.Done():
		t.Fatal("source sender did not join")
	}
	if !errors.Is(sender, ErrAuthority) || session.Active() {
		t.Fatal("lost authority or source suppression survived sender retirement", sender)
	}
	// Native join is independent; this synthetic queue test owns no Windows
	// hook or input. Feed its actual joined sender result through the wire.
	result := sourceCaptureFinal(ErrAuthority, context.Canceled, sender, false)
	if result == nil || sourceRetirementStatus(result) != 0 {
		t.Fatal("clean source owners became recovery-required", result)
	}
	service, child := net.Pipe()
	defer service.Close()
	finished := make(chan error, 1)
	go func() { finished <- finishWorkerReport(child, Report{}, result); _ = child.Close() }()
	report, received := readReport(service)
	<-finished
	_, received = receipt(report, received)
	if received == nil || serviceRecoveryFailure(received, nil) != nil {
		t.Fatal("joined pipe/authority failure stopped service or lost failed command", received)
	}
}

func TestCleanRetirementWithDisconnectedAgentCannotStopService(t *testing.T) {
	for _, packet := range []sourcePacket{{kind: sourceReply, op: sourceStop, reply: 1}, {kind: sourceRetired}} {
		agent, conn := net.Pipe()
		_ = agent.Close() // Winlogon transition may retire the medium agent pipe.
		defer conn.Close()
		writer := &sourceWriter{channel: &Channel{stream: conn}, binding: Binding{1}}
		result := sourceCaptureFinal(context.Canceled, inputdesktop.ErrInactive, context.Canceled, false)
		result = sourceRetirementDelivery(result, writer.write(packet, nil, true))
		if result == nil || errors.Is(result, ErrRecovery) {
			t.Fatal("lost agent notification became unknown native cleanup", result)
		}
		// The service receives a separate authenticated report. The failed agent
		// notification must not prevent that report or invent a native-error code.
		service, child := net.Pipe()
		done := make(chan error, 1)
		go func() { done <- finishWorkerReport(child, Report{}, result); _ = child.Close() }()
		report, received := readReport(service)
		_ = service.Close()
		<-done
		_, received = receipt(report, received)
		if serviceRecoveryFailure(received, nil) != nil || errors.Is(received, ErrRecovery) {
			t.Fatal("clean native report stopped capture service", received)
		}
		r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
		r.record("worker", "FAILED", serviceEndpointFailure(received, "capture"))
		failure := r.value.LastFailure
		if failure == nil || failure.Operation != "source-retirement-write" || failure.Win32Known || !r.value.Valid() {
			t.Fatal("delivery failure classification lost", failure)
		}
	}
}
