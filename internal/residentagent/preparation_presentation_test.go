package residentagent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestSeparateDemandPreparesDuringPresentationButInputStillJoinsRetirement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &privilegedLeaseInput{prepareDuringPresentation: true}
		prepares, cancels, connects := 0, 0, 0
		input.prepare = func(context.Context) error { prepares++; return nil }
		input.cursor.cancel = func() { cancels++ }
		input.cursor.done = make(chan struct{})
		input.connect = func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
			connects++
			return nil, errors.New("unexpected input")
		}
		// The old serial guard dropped this real hint while the spare expired.
		if accepted, err := input.prepareIdle(context.Background()); !accepted || err != nil || prepares != 1 || cancels != 0 {
			t.Fatal("fresh demand still waits for cursor restoration", accepted, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		err := input.open(ctx, application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now())
		if !errors.Is(err, context.DeadlineExceeded) || connects != 0 || cancels != 1 || input.cursor.done == nil {
			t.Fatal("unbound preparation bypassed native presentation retirement", err, connects, cancels)
		}
		close(input.cursor.done)
		input.closeErr = errors.New("cleanup failed")
		if accepted, err := input.prepareIdle(context.Background()); accepted || err != nil || prepares != 1 {
			t.Fatal("cleanup failure allowed another demand")
		}
	})
}
