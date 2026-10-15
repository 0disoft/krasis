package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
	"testing/iotest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestDesktopDiagnosticMirrorsEveryACKLayoutWithoutAddingIOOrChangingOutcome(t *testing.T) {
	for _, version := range []byte{1, 2, 3, 4, 5, 6, 7} {
		base := response{op: opMoveRead, sequence: 1, x: 3, y: -2}
		if version == 2 {
			base.timing = [3]byte{1, 2, 3}
		}
		if version >= 3 {
			base.timing, base.millis, base.measured = [3]byte{1, 1, 2}, [3]uint16{0, 0, 3}, 7
		}
		if version == 4 || version == 5 || version == 7 {
			base.applyDetail = &handoff.WorkerApplyProfile{Micros: [2]uint32{1000, 100}, Measured: 3}
		}
		if version == 5 || version == 7 {
			base.applyDetail.SubmissionCalls = 1
			base.applyDetail.SubmissionMicros = 10
		}
		if version >= 6 {
			base.status, base.certainty, base.failureStage = 1, 3, failureStageExecutorCompletion
		}
		var legacy bytes.Buffer
		if err := writeResponse(&legacy, base); err != nil || legacy.Bytes()[4] != version {
			t.Fatalf("fixture %d: %v", version, err)
		}
		for _, target := range []inputdesktop.DesktopClass{inputdesktop.DesktopDefault, inputdesktop.DesktopWinlogon} {
			value := base
			value.desktop = target
			writer := &cursorACKWriter{}
			if err := writeResponse(writer, value); err != nil || writer.writes != 1 || writer.Len() != legacy.Len() || writer.Bytes()[4] != version+7 {
				t.Fatalf("version %d adds IO or alters layout: %v", version, err)
			}
			packet := append([]byte(nil), writer.Bytes()...)
			if err := writeResponse(writer, response{op: opMove, sequence: 2}); err != nil {
				t.Fatal(err)
			}
			reader := iotest.OneByteReader(writer)
			got, err := readResponse(reader)
			if err != nil || got.desktop != target {
				t.Fatalf("version %d target lost: %+v %v", version, got, err)
			}
			got.desktop = 0
			var expected response
			expected, err = readResponse(bytes.NewReader(legacy.Bytes()))
			if err != nil || !reflect.DeepEqual(got, expected) {
				t.Fatalf("version %d changed input outcome: %+v != %+v", version, got, expected)
			}
			if next, err := readResponse(reader); err != nil || next.sequence != 2 {
				t.Fatalf("version %d shifted following ACK", version)
			}
			packet[7] |= 0xc0 // Unrecognized diagnostic target cannot invalidate a known input result.
			unknown, err := readResponse(bytes.NewReader(packet))
			if err != nil || unknown.desktop != 0 {
				t.Fatalf("version %d invalid metadata changed ACK: %v", version, err)
			}
			unknown.desktop = 0
			if !reflect.DeepEqual(unknown, expected) {
				t.Fatal("invalid diagnostic target changed outcome")
			}
		}
	}
}

func TestDesktopDiagnosticOnlyLogsTransitionsWithinLeaseBound(t *testing.T) {
	var codes []string
	c := &Client{}
	c.SetTimingObserver(func(code string) { codes = append(codes, code) })
	for i := 0; i < 100; i++ {
		c.observeDesktop(inputdesktop.DesktopDefault)
	}
	c.observeDesktop(inputdesktop.DesktopUnknown)
	c.observeDesktop(inputdesktop.DesktopDefault)
	if len(codes) != 1 || codes[0] != "installed_input_desktop_default" {
		t.Fatal("logged repeated or unknown observations", codes)
	}
	for i := 0; i < 100; i++ {
		c.observeDesktop(inputdesktop.DesktopClass(1 + i%2))
	}
	if len(codes) != desktopObservationLimit+1 || codes[len(codes)-1] != "installed_input_desktop_limit" {
		t.Fatal("desktop observation limit failed", len(codes))
	}
}

func TestClientCannotLogDesktopFromForeignACK(t *testing.T) {
	for _, match := range []bool{true, false} {
		server, conn := net.Pipe()
		client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
		if err != nil {
			t.Fatal(err)
		}
		var desktops []string
		client.SetTimingObserver(func(code string) {
			if code == "installed_input_desktop_winlogon" {
				desktops = append(desktops, code)
			}
		})
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer server.Close()
			request, err := readRequest(server)
			if err != nil {
				return
			}
			sequence := request.sequence
			if !match {
				sequence++
			}
			_ = writeResponse(server, response{op: request.op, sequence: sequence, desktop: inputdesktop.DesktopWinlogon})
		}()
		err = client.Move(context.Background(), 1, 1)
		<-done
		client.Close()
		if match {
			if err != nil || len(desktops) != 1 {
				t.Fatalf("matching target not observed: %v %v", err, desktops)
			}
		} else {
			certainty, known := application.InjectionCertaintyOf(err)
			if !known || certainty != application.InjectionOutcomeUnknown || len(desktops) != 0 {
				t.Fatalf("foreign target logged or input certainty changed: %v %v", err, desktops)
			}
		}
	}
}

func TestWorkerDesktopMetadataCannotSurviveAnUnexecutedScope(t *testing.T) {
	server, conn := net.Pipe()
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lease := testLease()
	timing := &workerTiming{}
	calls := 0
	execute := func(_ context.Context, work func() error) error {
		calls++
		if calls > 1 {
			return errors.New("scope rejected before input")
		}
		timing.desktop = inputdesktop.DesktopWinlogon
		return work()
	}
	done := make(chan error, 1)
	go func() {
		_, err := serve(ctx, server, &recordingInput{}, lease, beforeInputCertainty(measureExecution(func() error { return nil }, execute, timing)), execute, time.Now, timing)
		done <- err
	}()
	for sequence := uint64(1); sequence <= 2; sequence++ {
		packet, err := encodeRequest(request{op: opHeartbeat, sequence: sequence, binding: lease.Binding})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = conn.Write(packet[:]); err != nil {
			t.Fatal(err)
		}
		ack, err := readResponse(conn)
		if err != nil {
			t.Fatal(err)
		}
		if sequence == 1 && (ack.desktop != inputdesktop.DesktopWinlogon || ack.status != 0) {
			t.Fatalf("actual worker did not transmit target: %+v", ack)
		}
		if sequence == 2 && (ack.desktop != inputdesktop.DesktopUnknown || ack.status != 1 || ack.certainty != 1) {
			t.Fatalf("old target leaked into unexecuted scope: %+v", ack)
		}
	}
	if err := <-done; err == nil {
		t.Fatal("scope failure became success")
	}
}
