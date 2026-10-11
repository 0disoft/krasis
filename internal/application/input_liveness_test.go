package application

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestAppliedInputKeepsExactLeaseLiveWithoutHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		port := &recordingInputPort{}
		runtime := newTestRuntime(t, port)
		grantInbound(t, runtime, time.Now())
		for sequence := uint64(1); sequence <= 5; sequence++ {
			time.Sleep(time.Second)
			err := runtime.HandleInputActionWithLiveness(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: sequence}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, time.Now, 2*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if !runtime.LeaseWatch().Progress.Equal(time.Now()) || !runtime.Snapshot().Liveness.LastObserved.Equal(time.Now()) {
				t.Fatal("applied input did not renew both watchdogs")
			}
		}
		time.Sleep(2 * time.Second)
		faulted, err := runtime.CheckLiveness(context.Background(), time.Now(), 2*time.Second)
		if !faulted || err != nil || runtime.Snapshot().Control.State != control.StateLocal {
			t.Fatalf("silent peer retained lease: %v %v", faulted, err)
		}
	})
}

func TestInputProgressRejectsExpiredInvalidAndFailedInput(t *testing.T) {
	for _, kind := range []string{"expired", "sequence", "epoch", "session", "invalid", "injection"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				port := &recordingInputPort{}
				runtime := newTestRuntime(t, port)
				grantInbound(t, runtime, time.Now())
				envelope := control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}
				action := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
				switch kind {
				case "expired":
					time.Sleep(2 * time.Second)
				case "sequence":
					envelope.Sequence = 2
				case "epoch":
					envelope.Epoch = "other"
				case "session":
					envelope.SessionID = "other"
				case "invalid":
					action.DeltaX = 0
				case "injection":
					port.moveErr = errors.New("injection failed")
				}
				err := runtime.HandleInputActionWithLiveness(context.Background(), envelope, action, time.Now, 2*time.Second)
				if err == nil || runtime.Snapshot().Control.State != control.StateLocal || runtime.Snapshot().HasLiveness {
					t.Fatalf("failed input retained liveness: %v %s", err, runtime.Snapshot())
				}
				if kind == "expired" && (!errors.Is(err, ErrInputLivenessExpired) || len(port.moves) != 0) {
					t.Fatal("expired input reached adapter", err)
				}
			})
		})
	}
}

type slowProgressInput struct{ recordingInputPort }

func (input *slowProgressInput) Move(ctx context.Context, x, y int32) error {
	time.Sleep(time.Second)
	return input.recordingInputPort.Move(ctx, x, y)
}

func TestInputProgressUsesAdmissionTimeBeforeSlowInjection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runtime := newTestRuntime(t, &slowProgressInput{})
		start := time.Now()
		grantInbound(t, runtime, start)
		err := runtime.HandleInputActionWithLiveness(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, time.Now, 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if !runtime.LeaseWatch().Progress.Equal(start) || !runtime.Snapshot().Liveness.LastObserved.Equal(start) {
			t.Fatal("slow adapter received a fresh lease at completion")
		}
	})
}
