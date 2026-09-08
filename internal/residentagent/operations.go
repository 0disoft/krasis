package residentagent

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/truststore"
)

const DefaultSessionStopTimeout = localapi.DefaultSessionStopTimeout

var (
	ErrNotConfigured = errors.New("resident agent operation is not configured")
	ErrAlreadyActive = errors.New("resident agent session is already active")
)

type SessionRunner func(context.Context, string) error

type PairWorkflow interface {
	Handle(context.Context, localapi.OperationRequest) (localapi.OperationResult, error)
}

type trustStore interface {
	List() (truststore.Document, error)
	Revoke(string, time.Time) (truststore.PeerRecord, error)
}

type Operations struct {
	root      context.Context
	stopAgent context.CancelCauseFunc
	identity  productidentity.PublicIdentity
	runner    SessionRunner
	pairing   PairWorkflow
	trust     trustStore
	now       func() time.Time

	mu          sync.Mutex
	sessionStop context.CancelCauseFunc
	sessionDone chan struct{}
	peerAlias   string
	lastCode    string
	counters    map[string]uint64
}

func New(parent context.Context, identity productidentity.PublicIdentity, runner SessionRunner, pairing PairWorkflow, trust trustStore) (*Operations, error) {
	if parent == nil || identity.Fingerprint == "" || identity.Generation == 0 {
		return nil, errors.New("resident agent parent context and device identity are required")
	}
	root, stop := context.WithCancelCause(parent)
	return &Operations{
		root:      root,
		stopAgent: stop,
		identity:  identity,
		runner:    runner,
		pairing:   pairing,
		trust:     trust,
		now:       time.Now,
		lastCode:  "ready",
		counters:  map[string]uint64{},
	}, nil
}

func (operations *Operations) Trust(context.Context) (localapi.Trust, error) {
	result := localapi.Trust{SchemaVersion: localapi.TrustSchemaVersion, Peers: []localapi.TrustedPeer{}}
	if operations.trust == nil {
		return result, nil
	}
	document, err := operations.trust.List()
	if errors.Is(err, truststore.ErrStoreNotFound) {
		return result, nil
	}
	if err != nil {
		return localapi.Trust{}, err
	}
	result.Revision = document.Revision
	for _, peer := range document.Peers {
		result.Peers = append(result.Peers, localapi.TrustedPeer{
			Fingerprint: peer.Fingerprint, Alias: peer.Alias, Role: string(peer.Role),
			AllowInboundControl: peer.AllowInboundControl, Revoked: peer.RevokedAt != nil,
		})
	}
	return result, nil
}

func (operations *Operations) RevokeTrust(_ context.Context, request localapi.OperationRequest) (localapi.OperationResult, error) {
	if operations.trust == nil {
		return rejected("not_configured"), nil
	}
	document, err := operations.trust.List()
	if err != nil {
		if errors.Is(err, truststore.ErrStoreNotFound) {
			return rejected("peer_not_found"), nil
		}
		return localapi.OperationResult{}, err
	}
	alias := ""
	for _, peer := range document.Peers {
		if peer.Fingerprint == request.Fingerprint {
			alias = peer.Alias
			break
		}
	}
	operations.mu.Lock()
	active := operations.sessionDone != nil && operations.peerAlias == alias
	operations.mu.Unlock()
	if active {
		return rejected("active_session_requires_disconnect"), nil
	}
	if _, err := operations.trust.Revoke(request.Fingerprint, operations.now().UTC()); err != nil {
		switch {
		case errors.Is(err, truststore.ErrPeerNotFound):
			return rejected("peer_not_found"), nil
		case errors.Is(err, truststore.ErrPeerRevoked):
			return rejected("peer_already_revoked"), nil
		case errors.Is(err, truststore.ErrStoreBusy):
			return failed("trust_store_busy"), nil
		default:
			return localapi.OperationResult{}, err
		}
	}
	operations.mu.Lock()
	operations.recordLocked("trust_revoked")
	operations.mu.Unlock()
	return completed("trust_revoked"), nil
}

func (operations *Operations) Topology(ctx context.Context) (localapi.Topology, error) {
	trust, err := operations.Trust(ctx)
	if err != nil {
		return localapi.Topology{}, err
	}
	operations.mu.Lock()
	state, active := "LOCAL", operations.peerAlias
	if operations.sessionDone != nil {
		state = "CONNECTED"
	}
	operations.mu.Unlock()
	result := localapi.Topology{SchemaVersion: localapi.TopologySchemaVersion, State: state, Layout: "UNCONFIGURED", Peers: []localapi.TopologyPeer{}}
	for _, peer := range trust.Peers {
		if peer.Revoked {
			continue
		}
		result.Peers = append(result.Peers, localapi.TopologyPeer{
			Alias: peer.Alias, AllowInboundControl: peer.AllowInboundControl, Connected: peer.Alias == active && state == "CONNECTED",
		})
	}
	sort.Slice(result.Peers, func(left, right int) bool { return result.Peers[left].Alias < result.Peers[right].Alias })
	return result, nil
}

func (operations *Operations) Context() context.Context {
	if operations == nil {
		return nil
	}
	return operations.root
}

func (operations *Operations) Status(context.Context) (localapi.Status, error) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	state := "LOCAL"
	actions := []string{"PAIR", "SAFE_STOP"}
	if operations.sessionDone != nil {
		state = "CONNECTED"
		actions = []string{"DISCONNECT", "SAFE_STOP"}
	} else {
		actions = append(actions, "CONNECT")
	}
	return localapi.Status{
		SchemaVersion:      localapi.StatusSchemaVersion,
		State:              state,
		PeerAlias:          operations.peerAlias,
		Actions:            actions,
		DeviceFingerprint:  operations.identity.Fingerprint,
		IdentityGeneration: operations.identity.Generation,
	}, nil
}

func (operations *Operations) Connect(_ context.Context, request localapi.OperationRequest) (localapi.OperationResult, error) {
	if request.PeerAlias == "" {
		return rejected("invalid_request"), nil
	}
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if operations.runner == nil {
		operations.recordLocked("not_configured")
		return rejected("not_configured"), nil
	}
	if operations.sessionDone != nil {
		operations.recordLocked("already_active")
		return rejected("already_active"), nil
	}
	ctx, cancel := context.WithCancelCause(operations.root)
	done := make(chan struct{})
	operations.sessionStop = cancel
	operations.sessionDone = done
	operations.peerAlias = request.PeerAlias
	operations.recordLocked("connect_started")
	go func(alias string) {
		defer cancel(nil)
		_ = operations.runner(ctx, alias)
		operations.finishSession(done)
	}(request.PeerAlias)
	return completed("connect_started"), nil
}

func (operations *Operations) Disconnect(ctx context.Context, _ localapi.OperationRequest) (localapi.OperationResult, error) {
	if ctx == nil {
		return rejected("invalid_request"), nil
	}
	operations.mu.Lock()
	stop, done := operations.sessionStop, operations.sessionDone
	if stop == nil || done == nil {
		operations.recordLocked("no_active_session")
		operations.mu.Unlock()
		return rejected("no_active_session"), nil
	}
	stop(errors.New("local disconnect requested"))
	operations.mu.Unlock()
	shutdown, cancel := context.WithTimeout(ctx, DefaultSessionStopTimeout)
	defer cancel()
	select {
	case <-done:
		operations.mu.Lock()
		operations.recordLocked("disconnected")
		operations.mu.Unlock()
		return completed("disconnected"), nil
	case <-shutdown.Done():
		operations.mu.Lock()
		operations.recordLocked("shutdown_timeout")
		operations.mu.Unlock()
		return failed("shutdown_timeout"), nil
	}
}

func (operations *Operations) Pair(ctx context.Context, request localapi.OperationRequest) (localapi.OperationResult, error) {
	if operations.pairing == nil {
		operations.mu.Lock()
		operations.recordLocked("not_configured")
		operations.mu.Unlock()
		return rejected("not_configured"), nil
	}
	return operations.pairing.Handle(ctx, request)
}

func (operations *Operations) Diagnostics(context.Context) (localapi.Diagnostics, error) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	counters := make(map[string]uint64, len(operations.counters))
	for key, value := range operations.counters {
		counters[key] = value
	}
	return localapi.Diagnostics{
		SchemaVersion: localapi.DiagnosticsSchemaVersion,
		State:         operations.lastCode,
		Counters:      counters,
	}, nil
}

func (operations *Operations) SafeStop(context.Context) (localapi.OperationResult, error) {
	operations.mu.Lock()
	operations.recordLocked("safe_stop_requested")
	operations.mu.Unlock()
	operations.stopAgent(errors.New("local safe stop requested"))
	return completed("safe_stop_requested"), nil
}

func (operations *Operations) Close(ctx context.Context) error {
	if operations == nil {
		return nil
	}
	operations.stopAgent(context.Canceled)
	result, err := operations.Disconnect(ctx, localapi.OperationRequest{})
	if err != nil {
		return err
	}
	if result.Code == "no_active_session" || result.Status == "COMPLETED" {
		return nil
	}
	return errors.New(result.Code)
}

func (operations *Operations) finishSession(done chan struct{}) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if operations.sessionDone == done {
		operations.sessionStop = nil
		operations.sessionDone = nil
		operations.peerAlias = ""
		operations.recordLocked("session_stopped")
	}
	// Publish completion only after clearing the session, and wake every waiter.
	close(done)
}

func (operations *Operations) recordLocked(code string) {
	operations.lastCode = code
	operations.counters[code]++
}

func completed(code string) localapi.OperationResult {
	return localapi.OperationResult{SchemaVersion: localapi.ResultSchemaVersion, Status: "COMPLETED", Code: code}
}

func rejected(code string) localapi.OperationResult {
	return localapi.OperationResult{SchemaVersion: localapi.ResultSchemaVersion, Status: "REJECTED", Code: code}
}

func failed(code string) localapi.OperationResult {
	return localapi.OperationResult{SchemaVersion: localapi.ResultSchemaVersion, Status: "FAILED", Code: code}
}
