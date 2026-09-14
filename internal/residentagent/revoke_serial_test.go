package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/truststore"
	"testing"
	"time"
)

type blockedRevokeStore struct {
	stubTrustStore
	entered, release chan struct{}
}

func (s *blockedRevokeStore) Revoke(fp string, now time.Time) (truststore.PeerRecord, error) {
	close(s.entered)
	<-s.release
	return s.stubTrustStore.Revoke(fp, now)
}

func TestRevocationHoldsSessionLockThroughMutation(t *testing.T) {
	s := &blockedRevokeStore{stubTrustStore: stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Fingerprint: "peer", Alias: "peer"}}}}, entered: make(chan struct{}), release: make(chan struct{})}
	ops := testOperationsWithTrust(t, s)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ops.RevokeTrust(context.Background(), localapi.OperationRequest{Fingerprint: "peer"})
	}()
	select {
	case <-s.entered:
	case <-time.After(time.Second):
		t.Fatal("revocation did not reach store")
	}
	unlocked := ops.mu.TryLock()
	if unlocked {
		ops.mu.Unlock()
	}
	close(s.release)
	<-done
	if unlocked {
		t.Fatal("connection state could change during revocation")
	}
}
