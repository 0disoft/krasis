package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type exitCheckedRelease struct {
	*recordingInput
	exited <-chan struct{}
}

func (release exitCheckedRelease) Release(ctx context.Context, token control.PressedToken) error {
	select {
	case <-release.exited:
		return release.recordingInput.Release(ctx, token)
	default:
		return errors.New("key-up preceded exact native child exit")
	}
}

// Real hidden input child, simulated injection only. The authenticated worker
// retains its pressed state while a native stand-in ignores cancellation.
func TestIsolatedWorkerRecoversHeldInputAfterNativeChildStall(t *testing.T) {
	process, client := startPortableFixture(t, "hang")
	recovery := &recordingInput{}
	input := newIsolatedInput(client, process, exitCheckedRelease{recovery, process.Done()})
	server, remote := net.Pipe()
	defer remote.Close()
	lease := testLease()
	done := make(chan finish, 1)
	go func() {
		report, err := serve(context.Background(), server, input, lease, testExecute, testExecute, time.Now)
		done <- finish{report, err}
	}()
	ack := send(t, remote, request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}})
	if ack[6] != 0 {
		t.Fatal("simulated down was not acknowledged")
	}
	if err := remote.SetDeadline(time.Now().Add(8 * time.Second)); err != nil {
		t.Fatal(err)
	}
	data, err := encodeRequest(request{op: opMove, sequence: 2, binding: lease.Binding, x: 1})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := remote.Write(data[:]); err != nil {
		t.Fatal(err)
	}
	response, err := readResponse(remote)
	if err == nil && (response.status == 0 || response.certainty != 3) {
		t.Fatal("native stall became success or unapplied", response)
	}
	select {
	case result := <-done:
		if result.err == nil || result.report.Recovery.Released != 1 || result.report.Recovery.Remaining != 0 || !input.recovering {
			t.Fatal("independent journal did not recover after exact retirement", result, input.recovering)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("native stall blocked worker retirement")
	}
	if time.Since(started) > 8*time.Second {
		t.Fatal("native stall exceeded bounded shutdown")
	}
	if recovery.pressed != 0 || recovery.released != 1 {
		t.Fatal("recovery owner injected a down or missed key-up", recovery.pressed, recovery.released)
	}
}

func TestIsolatedWorkerRefusesRecoveryWithoutFullRetirement(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "held"}[held], func(t *testing.T) {
			recovery := &recordingInput{}
			failure := errors.New("native retirement pending")
			live := &Client{localExchange: func(context.Context, request, func(response)) error { return nil }}
			input := &isolatedInput{Client: live, recovery: recovery, retire: func() (bool, error) { return false, failure }}
			server, remote := net.Pipe()
			defer remote.Close()
			lease := testLease()
			done := make(chan finish, 1)
			go func() {
				report, err := serve(context.Background(), server, input, lease, testExecute, testExecute, time.Now)
				done <- finish{report, err}
			}()
			remaining := 0
			if held {
				send(t, remote, request{op: opPress, sequence: 1, binding: lease.Binding, token: control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}})
				remaining = 1
			}
			_ = remote.Close()
			result := await(t, done)
			if !errors.Is(result.err, ErrRecovery) || !errors.Is(result.err, failure) || input.recovering || recovery.released != 0 || result.report.Recovery.Remaining != remaining {
				t.Fatal("uncertain native lifetime permitted key-up", result)
			}
		})
	}
}
