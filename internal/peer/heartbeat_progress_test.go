package peer

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type heartbeatProgressEvent struct {
	stage HeartbeatStage
	at    time.Time
}
type heartbeatProgressRecorder struct {
	sessionLivenessObserver
	send, receive []heartbeatProgressEvent
}

func (r *heartbeatProgressRecorder) ObserveHeartbeatSend(stage HeartbeatStage, at time.Time) {
	r.send = append(r.send, heartbeatProgressEvent{stage, at})
}
func (r *heartbeatProgressRecorder) ObserveHeartbeatReceive(stage HeartbeatStage, at time.Time) {
	r.receive = append(r.receive, heartbeatProgressEvent{stage, at})
}

type progressFrameWriter struct {
	delay time.Duration
	err   error
}

func (w progressFrameWriter) WriteFrameContext(context.Context, transport.Frame) error {
	time.Sleep(w.delay)
	return w.err
}

func TestObservedHeartbeatSeparatesCoordinatorWaitFromWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runtime, _ := newInboundRuntime(t)
		c := testWriteCoordinator(t, progressFrameWriter{delay: 40 * time.Millisecond})
		holding, release, held := make(chan struct{}), make(chan struct{}), make(chan struct{})
		go func() {
			_ = c.Execute(context.Background(), func(FrameWriter) error { close(holding); <-release; return nil })
			close(held)
		}()
		<-holding
		r := &heartbeatProgressRecorder{}
		done := make(chan error, 1)
		go func() {
			_, err := PulseLivenessObserved(context.Background(), c, runtime, fixedClock(time.Unix(101, 0)), 3*time.Second, r)
			done <- err
		}()
		synctest.Wait()
		if len(r.send) != 1 || r.send[0].stage != HeartbeatStart {
			t.Fatal("queued pulse pretended to acquire/write", r.send)
		}
		time.Sleep(80 * time.Millisecond)
		close(release)
		<-held
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if len(r.send) != 4 || r.send[1].stage != HeartbeatGate || r.send[2].stage != HeartbeatWork || r.send[3].stage != HeartbeatComplete ||
			r.send[1].at.Sub(r.send[0].at) != 80*time.Millisecond || r.send[3].at.Sub(r.send[2].at) != 40*time.Millisecond {
			t.Fatal("wait/write boundaries mixed", r.send)
		}
	})
}

func TestObservedHeartbeatNeverReportsSkippedExpiredOrFailedWriteAsSent(t *testing.T) {
	for _, outcome := range []string{"inactive", "expired", "write", "canceled"} {
		t.Run(outcome, func(t *testing.T) {
			runtime, _ := newInboundRuntime(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := &recordingFrameWriter{}
			clock := fixedClock(time.Unix(101, 0))
			switch outcome {
			case "inactive":
				runtime = newLocalRuntime(t)
			case "expired":
				clock = fixedClock(time.Unix(103, 0))
			case "write":
				w.err = errors.New("write failed")
			case "canceled":
				cancel()
			}
			r := &heartbeatProgressRecorder{}
			p, err := PulseLivenessObserved(ctx, testWriteCoordinator(t, w), runtime, clock, 3*time.Second, r)
			if p.Sent {
				t.Fatal("failed/skipped heartbeat certified a write")
			}
			terminals := 0
			for _, e := range r.send {
				if e.stage >= HeartbeatComplete {
					terminals++
					if e.stage == HeartbeatComplete {
						t.Fatal("false success")
					}
				}
			}
			if terminals != 1 || outcome == "inactive" && (err != nil || r.send[len(r.send)-1].stage != HeartbeatSkipped) || outcome != "inactive" && err == nil {
				t.Fatal("missing/duplicate terminal outcome", err, r.send)
			}
		})
	}
}

func TestHeartbeatReceivePhasesDoNotAcceptExpiredOrMalformedFrames(t *testing.T) {
	for _, outcome := range []string{"accepted", "expired", "malformed"} {
		t.Run(outcome, func(t *testing.T) {
			runtime, _ := newInboundRuntime(t)
			payload, _ := transport.EncodeHeartbeatPayload("session-1")
			clock := fixedClock(time.Unix(101, 0))
			if outcome == "expired" {
				clock = fixedClock(time.Unix(103, 0))
			}
			if outcome == "malformed" {
				payload = []byte{0}
			}
			conn := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageHeartbeat, Payload: payload}}}
			r := &heartbeatProgressRecorder{}
			err := RunSessionWithLifecycleObserved(context.Background(), conn, testWriteCoordinator(t, &recordingFrameWriter{}), runtime, nil, nil, clock, 3*time.Second, r)
			if err == nil {
				t.Fatal("session lost EOF/failure")
			}
			if len(r.receive) != 4 {
				t.Fatal("missing receive phase", r.receive)
			}
			for i := 0; i < 3; i++ {
				if r.receive[i].stage != HeartbeatStage(i) {
					t.Fatal("receive stages reordered", r.receive)
				}
			}
			want := HeartbeatFailed
			if outcome == "accepted" {
				want = HeartbeatComplete
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatal(err)
				}
			}
			if r.receive[3].stage != want {
				t.Fatal("invalid heartbeat classified as accepted", r.receive)
			}
			if runtime.Snapshot().Control.State != control.StateLocal {
				t.Fatal("EOF/failure skipped cleanup")
			}
		})
	}
}
