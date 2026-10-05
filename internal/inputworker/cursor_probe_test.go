package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
)

type probingInput struct {
	placingInput
	probes int
}

func (p *probingInput) ProbeCursorPlacement(context.Context, int32, int32) (application.CursorPlacement, error) {
	p.probes++
	return application.CursorPlacement{TargetX: -3, TargetY: 4, X: -2, Y: 4,
		Observation: &application.CursorPlacementObservation{Known: 63, Flags: application.CursorTargetAdjusted | application.CursorPhysicalMatchesReadback}}, nil
}

func TestPlacementProbeUsesOneScopeAndOnePlacement(t *testing.T) {
	server, conn := net.Pipe()
	input := &probingInput{}
	done := make(chan finish, 1)
	scopes := 0
	go func() {
		r, err := serve(context.Background(), server, input, testLease(), func(ctx context.Context, f func() error) error { scopes++; return testExecute(ctx, f) }, testExecute, time.Now)
		done <- finish{r, err}
	}()
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	c := handoff.New(func(handoff.Summary) {})
	ctx := handoff.Begin(handoff.WithSession(context.Background(), c), "receiver", "s", "e", time.Now())
	handoff.Backend(ctx, "installed")
	p, err := client.PlaceCursor(ctx, 0, 0)
	if err != nil || p.Observation == nil || p.Observation.Known != 63 || input.probes != 1 || input.places != 0 {
		t.Fatal("probe missing or placement repeated", err)
	}
	handoff.Finish(ctx, "error")
	_ = client.Close()
	_ = await(t, done)
	if scopes != 1 {
		t.Fatal("probe added a scope")
	}
}

func TestProbeSuffixKeepsFollowingACKAlignedAndDropsInvalidMetadata(t *testing.T) {
	for _, profiled := range []bool{false, true} {
		for _, invalid := range []bool{false, true} {
			var stream bytes.Buffer
			ack := response{op: opCursorPlaceProbe, sequence: 1, x: -2, y: 4, targetX: -3, targetY: 4,
				observation: &application.CursorPlacementObservation{Known: 3, Flags: 1}}
			if profiled {
				ack.applyDetail = &handoff.WorkerApplyProfile{Measured: 1, SubmissionCalls: 1}
			}
			if err := writeResponse(&stream, ack); err != nil {
				t.Fatal(err)
			}
			original := append([]byte(nil), stream.Bytes()...)
			if invalid {
				stream.Bytes()[stream.Len()-1] = 128
			}
			_ = writeResponse(&stream, response{op: opMove, sequence: 2})
			got, err := readResponse(&stream)
			if err != nil || got.x != -2 || got.targetX != -3 || (got.observation == nil) != invalid {
				t.Fatal("diagnostic changed placement ACK", err)
			}
			if next, err := readResponse(&stream); err != nil || next.sequence != 2 {
				t.Fatal("ACK alignment lost", err)
			}
			if _, err := readResponse(bytes.NewReader(original[:len(original)-1])); !errors.Is(err, ErrProtocol) {
				t.Fatal("truncated probe accepted")
			}
		}
	}
	data, err := encodeRequest(request{op: opCursorPlaceProbe, sequence: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := readRequest(bytes.NewReader(data[:])); err != nil || got.op != opCursorPlaceProbe {
		t.Fatal("zero target probe rejected", err)
	}
}
