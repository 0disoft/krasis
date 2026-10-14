//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestSourceWriterCannotPreemptServiceReadinessWithEvents(t *testing.T) {
	w := &sourceWriter{}
	for _, kind := range []byte{sourceBoundary, sourceAction, sourceReturn} {
		if err := w.send(sourcePacket{kind: kind}, nil); err != nil || w.sequence != 0 {
			t.Fatal("event reached an unstarted stream", err)
		}
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
