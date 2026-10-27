package inputworker

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestIsolatedMoveSubmissionReachesNativeChildWithoutExtraExchange(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			native := &submissionInput{}
			admissions := 0
			transport := &entryTransport{handle: PortableInputHandler(native, func(_ context.Context, work func() error) error {
				admissions++
				time.Sleep(100 * time.Millisecond) // Scope/RPC delay is outside SendInput.
				return work()
			})}
			client, err := newPortableClient(context.Background(), transport)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			admissions = 0
			var pressed control.PressedState
			var ack response
			err = applyCommand(context.Background(), &isolatedInput{Client: client}, &pressed, request{op: opMoveRead, x: 1, applyDetail: true, submissionDetail: enabled}, &ack)
			if err != nil || admissions != 1 || len(transport.commands) != 1 || (native.probe != nil) != enabled {
				t.Fatal("missing probe or duplicate native transaction", err, admissions, transport.commands)
			}
			if !ack.applyDetail.Valid() || ack.applyDetail.Micros[0] != 134000 || (ack.applyDetail.SubmissionCalls == 1) != enabled || enabled && ack.applyDetail.SubmissionMicros != 30000 {
				t.Fatal("native call conflated with IPC/scope or legacy request changed", ack.applyDetail)
			}
		})
	}
}
