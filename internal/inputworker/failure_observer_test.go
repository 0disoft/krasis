package inputworker

import (
	"context"
	"net"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

func TestFailureObserverPanicPreservesInputCertaintyAndChannelRetirement(t *testing.T) {
	for _, example := range []struct {
		name      string
		ack       *response
		certainty application.InjectionCertainty
	}{
		{"lost_ack", nil, application.InjectionOutcomeUnknown},
		{"rejected", &response{status: 1, certainty: 1, requested: 1}, application.InjectionNotApplied},
		{"partial", &response{status: 1, certainty: 2, requested: 2, inserted: 1}, application.InjectionPartiallyApplied},
	} {
		t.Run(example.name, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			client.SetTimingObserver(func(string) { panic("diagnostic observer failed") })
			done := make(chan error, 1)
			go func() {
				defer server.Close()
				command, err := readRequest(server)
				if err == nil && example.ack != nil {
					ack := *example.ack
					ack.sequence, ack.op = command.sequence, command.op
					err = writeResponse(server, ack)
				}
				done <- err
			}()
			var diagnosticPanic any
			func() {
				defer func() { diagnosticPanic = recover() }()
				err = client.Press(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65})
			}()
			if workerErr := <-done; workerErr != nil {
				t.Fatal(workerErr)
			}
			certainty, ok := application.InjectionCertaintyOf(err)
			if diagnosticPanic != nil || !ok || certainty != example.certainty || !client.failed {
				t.Fatal("diagnostic panic hid the input result or bypassed retirement", diagnosticPanic, err, certainty, client.failed)
			}
			sequence := client.sequence
			err = client.Move(context.Background(), 1, 0)
			if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionNotApplied || client.sequence != sequence {
				t.Fatal("diagnostic failure allowed another write on a failed channel", err)
			}
		})
	}
}
