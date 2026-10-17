package truststore

import (
	"bytes"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

// PeerPolicyMonitor owns one immutable, validated certificate for one session.
// Only cryptographic work is reused: policy, role, revocation, file contents and
// ACLs are checked under a fresh shared lock, including on every lease request.
// It retains no file handles or authorization verdicts and is concurrency-safe.
type PeerPolicyMonitor struct {
	store    *Store
	peer     transport.AuthenticatedPeerContext
	approved []byte
}

func (store *Store) NewPeerPolicyMonitor(peer transport.AuthenticatedPeerContext, now time.Time) (*PeerPolicyMonitor, error) {
	leaf, err := store.PeerKeyAnchor(peer, now)
	if err != nil {
		return nil, err
	}
	return &PeerPolicyMonitor{store: store, peer: peer, approved: bytes.Clone(leaf.Raw)}, nil
}

func (monitor *PeerPolicyMonitor) CheckAnchor(now time.Time, trace *ReadTrace) error {
	_, err := monitor.currentPolicy(now, trace)
	return err
}

func (monitor *PeerPolicyMonitor) InboundControlAllowed(peer transport.AuthenticatedPeerContext, now time.Time) (bool, error) {
	if monitor == nil || peer != monitor.peer {
		return false, ErrPeerCertificateUnavailable
	}
	return monitor.currentPolicy(now, nil)
}

func (monitor *PeerPolicyMonitor) currentPolicy(now time.Time, trace *ReadTrace) (bool, error) {
	if monitor == nil || now.IsZero() {
		return false, ErrPeerCertificateUnavailable
	}
	allowed := false
	err := monitor.store.withPeerRecordCertificate(monitor.peer, trace, func(record PeerRecord, current []byte) error {
		trace.enterStep("certificate_match")
		if !bytes.Equal(current, monitor.approved) {
			return ErrPeerCertificateUnavailable
		}
		allowed = record.AllowInboundControl
		return nil
	})
	return allowed, err
}

func (store *Store) NewKeyAnchorMonitor(peer transport.AuthenticatedPeerContext, now time.Time) (func(time.Time, *ReadTrace) error, error) {
	monitor, err := store.NewPeerPolicyMonitor(peer, now)
	if err != nil {
		return nil, err
	}
	return monitor.CheckAnchor, nil
}
