package residentagent

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peer"
)

func TestIgnoredControlProgressDoesNotWaitForOperationsOwner(t *testing.T) {
	o := &Operations{root: context.Background(), sessionState: "CONNECTED"}
	observer := &sessionObserver{operations: o}
	o.mu.Lock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for event := peer.ControlFlowBoundaryAdmitted; event <= peer.ControlFlowReadAfterLocalInterrupt; event++ {
			if boundaryFlowCode(event) == "" && event != peer.ControlFlowCaptureActivationSucceeded {
				observer.ObserveControlFlow(event)
			}
		}
	}()
	select {
	case <-done:
		o.mu.Unlock()
	case <-time.After(time.Second):
		o.mu.Unlock()
		<-done
		t.Fatal("lease progress with no UI effect waited for the shared state lock")
	}
}

func TestCaptureSuccessStillClearsOnlyCurrentSessionNotice(t *testing.T) {
	o := &Operations{root: context.Background(), sessionState: "CONNECTED", sessionDone: make(chan struct{}), boundaryBlocked: "PRESSED_INPUT", boundaryBlockedAt: time.Now()}
	observer := &sessionObserver{operations: o, done: o.sessionDone}
	observer.ObserveControlFlow(peer.ControlFlowCaptureActivationSucceeded)
	if o.boundaryBlocked != "" || !o.boundaryBlockedAt.IsZero() {
		t.Fatal("capture activation left the old blocked notice")
	}
	o.boundaryBlocked, o.boundaryBlockedAt = "PRESSED_INPUT", time.Now()
	o.sessionDone = make(chan struct{})
	observer.ObserveControlFlow(peer.ControlFlowCaptureActivationSucceeded)
	if o.boundaryBlocked == "" || o.boundaryBlockedAt.IsZero() {
		t.Fatal("retired capture callback changed the new session notice")
	}
}
