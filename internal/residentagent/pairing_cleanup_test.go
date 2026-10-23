package residentagent

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type blockedInvitationClose struct {
	entered, release chan struct{}
	calls            atomic.Int32
}

func (c *blockedInvitationClose) Close() error {
	if c.calls.Add(1) == 1 {
		close(c.entered)
	}
	<-c.release
	return nil
}

func TestPINJobRemainsActiveUntilInvitationClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node := newFlowNode(t, ctx)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	closer := &blockedInvitationClose{entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	unblock := func() { release.Do(func() { close(closer.release) }) }
	t.Cleanup(unblock)
	node.pairing.listen = func(string, string) (net.Listener, error) { return listener, nil }
	node.pairing.advertise = func(context.Context, string, int) (io.Closer, error) { return closer, nil }
	allow := true
	invite, err := node.pairing.Handle(ctx, localapi.OperationRequest{PairingAction: "PIN_LISTEN", PeerAlias: "peer", AllowInboundControl: &allow})
	if err != nil || invite.Code != "pin_waiting" {
		t.Fatalf("start: code=%s error=%v", invite.Code, err)
	}
	result, err := node.pairing.Handle(ctx, localapi.OperationRequest{PairingAction: "CANCEL", PairingID: invite.PairingID})
	if err != nil || result.Code != "pairing_cancelled" {
		t.Fatalf("cancel: code=%s error=%v", result.Code, err)
	}
	select {
	case <-closer.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("invitation cleanup was not requested")
	}
	shutdown, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	err = node.pairing.Close(shutdown)
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Close returned %v before invitation cleanup completed", err)
	}
	if !node.pairing.Active() {
		t.Error("pairing was reported idle while its invitation still owned the discovery socket")
	}
	unblock()
	shutdown, stop = context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if err = node.pairing.Close(shutdown); err != nil {
		t.Fatal(err)
	}
	if node.pairing.Active() || closer.calls.Load() != 1 {
		t.Fatalf("active=%t close calls=%d", node.pairing.Active(), closer.calls.Load())
	}
}
