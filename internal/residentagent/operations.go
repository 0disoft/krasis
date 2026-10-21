package residentagent

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/truststore"
	"github.com/0disoft/krasis/internal/wininput"
)

const DefaultSessionStopTimeout = localapi.DefaultSessionStopTimeout

var (
	ErrNotConfigured = errors.New("resident agent operation is not configured")
	ErrAlreadyActive = errors.New("resident agent session is already active")
)

// SessionRunner must report the shared supervisor's actual state. Starting the
// runner alone is not evidence of a peer connection.
type SessionRunner func(context.Context, string, agent.SupervisorObserver) error

type PairWorkflow interface {
	Handle(context.Context, localapi.OperationRequest) (localapi.OperationResult, error)
}

type trustStore interface {
	List() (truststore.Document, error)
	Revoke(string, time.Time) (truststore.PeerRecord, error)
}

type Operations struct {
	localAppData           string
	updates                *peerupdate.Manager
	updateApplying         bool
	updateTargetGeneration int
	updateTargetRevision   string
	releaseClaimID         string
	uiReadyAt              time.Time
	uiReadyPID             uint32
	log                    *productlog.Logger
	root                   context.Context
	stopAgent              context.CancelCauseFunc
	identity               productidentity.PublicIdentity
	runner                 SessionRunner
	preflight              func(string) error
	credentials            *productidentity.Store
	pairing                PairWorkflow
	trust                  trustStore
	now                    func() time.Time

	mu                   sync.Mutex
	sessionStop          context.CancelCauseFunc
	sessionTransition    context.CancelCauseFunc
	sessionSourceRefresh *sourceRefreshSlot
	desktopChanging      bool
	sessionDone          chan struct{}
	guardExitDone        <-chan struct{}
	sessionState         string
	inputBackend         string
	boundaryBlocked      string
	boundaryBlockedAt    time.Time
	sessionFaulted       bool
	sessionOutcome       *sessionOutcome
	peerAlias            string
	lastStopReason       string
	manualStop           bool
	resumeState          string
	lastCode             string
	counters             map[string]uint64
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
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if operations.updateApplying {
		return rejected("update_in_progress"), nil
	}
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
	active := operations.sessionDone != nil && operations.peerAlias == alias
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
	operations.recordLocked("trust_revoked")
	return completed("trust_revoked"), nil
}

func (operations *Operations) Topology(ctx context.Context) (localapi.Topology, error) {
	trust, err := operations.Trust(ctx)
	if err != nil {
		return localapi.Topology{}, err
	}
	operations.mu.Lock()
	state, active := operations.stateLocked(), operations.peerAlias
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
	state := operations.stateLocked()
	actions := []string{"PAIR", "SAFE_STOP"}
	if operations.sessionDone != nil {
		actions = []string{"DISCONNECT", "SAFE_STOP"}
	} else if operations.sessionFaulted {
		actions = []string{"SAFE_STOP"}
	} else {
		actions = append(actions, "CONNECT")
	}
	return localapi.Status{
		SchemaVersion:      localapi.StatusSchemaVersion,
		State:              state,
		ResumeState:        operations.resumeState,
		InputBackend:       operations.inputBackend,
		BoundaryBlocked:    operations.boundaryNoticeLocked(),
		PeerAlias:          operations.peerAlias,
		StopReason:         operations.lastStopReason,
		Actions:            actions,
		DeviceFingerprint:  operations.identity.Fingerprint,
		IdentityGeneration: operations.identity.Generation,
	}, nil
}

func (operations *Operations) Connect(requestContext context.Context, request localapi.OperationRequest) (localapi.OperationResult, error) {
	if requestContext == nil || request.PeerAlias == "" {
		return rejected("invalid_request"), nil
	}
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if requestContext.Err() != nil || operations.root.Err() != nil {
		return rejected("operation_cancelled"), nil
	}
	if request.AutomaticResume && operations.manualStop {
		return rejected("resume_cancelled"), nil
	}
	if request.AutomaticResume && operations.localAppData != "" {
		resume, err := appsettings.LoadResume(operations.localAppData)
		if err != nil {
			operations.recordLocked("resume_read_failed")
			return rejected("resume_unavailable"), nil
		}
		if resume.ManualStop {
			operations.manualStop = true
			return rejected("resume_cancelled"), nil
		}
	}
	if operations.updateApplying {
		return rejected("update_in_progress"), nil
	}
	if operations.sessionFaulted {
		return rejected("session_failed"), nil
	}
	if workflow, ok := operations.pairing.(interface{ Active() bool }); ok && workflow.Active() {
		return rejected("pairing_already_active"), nil
	}
	if operations.runner == nil {
		operations.recordLocked("not_configured")
		return rejected("not_configured"), nil
	}
	if operations.sessionDone != nil {
		operations.recordLocked("already_active")
		return rejected("already_active"), nil
	}
	if request.Fingerprint != "" {
		if operations.trust == nil {
			return rejected("selected_peer_changed"), nil
		}
		document, err := operations.trust.List()
		if err != nil {
			return rejected("credential_unavailable"), nil
		}
		matches := 0
		for _, peer := range document.Peers {
			if peer.Alias == request.PeerAlias && peer.RevokedAt == nil {
				matches++
				if peer.Fingerprint != request.Fingerprint {
					return rejected("selected_peer_changed"), nil
				}
			}
		}
		if matches != 1 {
			return rejected("selected_peer_changed"), nil
		}
	}
	if operations.preflight != nil {
		if err := operations.preflight(request.PeerAlias); err != nil {
			code := "credential_unavailable"
			if errors.Is(err, truststore.ErrConnectionNotConfigured) {
				code = "connection_not_configured"
			}
			if errors.Is(err, truststore.ErrPeerNotFound) || errors.Is(err, truststore.ErrStoreNotFound) {
				code = "peer_not_found"
			}
			if errors.Is(err, truststore.ErrAliasAmbiguous) {
				code = "peer_alias_ambiguous"
			}
			operations.recordLocked(code)
			return rejected(code), nil
		}
	}
	ctx, cancel := context.WithCancelCause(operations.root)
	ctx = localapi.WithPeerFingerprint(ctx, request.Fingerprint)
	done := make(chan struct{})
	outcome := &sessionOutcome{}
	operations.sessionOutcome = outcome
	operations.sessionStop = cancel
	operations.sessionDone = done
	operations.sessionState = "CONNECTING"
	operations.inputBackend = "UNKNOWN"
	operations.boundaryBlocked, operations.boundaryBlockedAt = "", time.Time{}
	operations.peerAlias = request.PeerAlias
	operations.lastStopReason = ""
	operations.manualStop = false
	operations.recordLocked("connect_started")
	resumeSaved := operations.rememberConnectionLocked(request.PeerAlias, request.Fingerprint)
	go func(alias string) {
		defer cancel(nil)
		inspect := boundedProtectedInputInspection(wininput.ProtectedInputActive)
		err := operations.runWithProtectedRecovery(ctx, alias, &sessionObserver{operations: operations, done: done}, func() bool {
			active, err := inspect(ctx)
			return active || err != nil // An unknown/late observation cannot rearm.
		}, agent.Wait)
		displayChanged := ctx.Err() == nil && displayChangeOnly(err)
		operations.finishSession(done, outcome, err, (ctx.Err() != nil && cancellationOnly(err, context.Cause(ctx))) || peerDisconnectOnly(err) || protectedInputOnly(err), displayChanged)
	}(request.PeerAlias)
	if !resumeSaved {
		return completed("connect_started_resume_save_failed"), nil
	}
	return completed("connect_started"), nil
}

func (operations *Operations) Disconnect(ctx context.Context, request localapi.OperationRequest) (localapi.OperationResult, error) {
	return operations.disconnect(ctx, !request.MaintenanceStop)
}

func (operations *Operations) disconnect(ctx context.Context, forget bool) (localapi.OperationResult, error) {
	if ctx == nil {
		return rejected("invalid_request"), nil
	}
	operations.mu.Lock()
	var resumeErr error
	if forget && operations.localAppData != "" {
		resumeErr = appsettings.SaveResume(operations.localAppData, appsettings.Resume{ManualStop: true})
		if resumeErr != nil {
			operations.resumeState = "FAILED"
			operations.recordLocked("resume_save_failed")
		} else {
			operations.resumeState = "SAVED"
		}
	}
	operations.lastStopReason = ""
	operations.manualStop = operations.manualStop || forget
	stop, done := operations.sessionStop, operations.sessionDone
	outcome := operations.sessionOutcome
	if stop == nil || done == nil {
		operations.recordLocked("no_active_session")
		operations.mu.Unlock()
		if resumeErr != nil {
			return failed("resume_save_failed"), nil
		}
		return rejected("no_active_session"), nil
	}
	operations.sessionState = "DISCONNECTING"
	operations.log.Event("disconnect_requested")
	stop(errLocalDisconnect)
	operations.mu.Unlock()
	shutdown, cancel := context.WithTimeout(ctx, DefaultSessionStopTimeout)
	defer cancel()
	select {
	case <-done:
		if outcome != nil && outcome.failed {
			return failed("session_failed"), nil
		}
		operations.mu.Lock()
		operations.recordLocked("disconnected")
		operations.mu.Unlock()
		if resumeErr != nil {
			return failed("resume_save_failed"), nil
		}
		return completed("disconnected"), nil
	case <-shutdown.Done():
		operations.mu.Lock()
		if operations.sessionDone == done {
			operations.sessionFaulted = true
		}
		operations.recordLocked("shutdown_timeout")
		operations.mu.Unlock()
		return failed("shutdown_timeout"), nil
	}
}

func (operations *Operations) Pair(ctx context.Context, request localapi.OperationRequest) (localapi.OperationResult, error) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if operations.updateApplying {
		return rejected("update_in_progress"), nil
	}
	if operations.sessionFaulted {
		return rejected("session_failed"), nil
	}
	if operations.sessionDone != nil {
		return rejected("active_session_requires_disconnect"), nil
	}
	if operations.pairing == nil {
		operations.recordLocked("not_configured")
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
	if operations.log.Failed() {
		counters["log_incomplete"] = 1
	}
	return localapi.Diagnostics{
		SchemaVersion: localapi.DiagnosticsSchemaVersion,
		State:         operations.lastCode,
		Counters:      counters,
	}, nil
}

func (operations *Operations) SafeStop(context.Context) (localapi.OperationResult, error) {
	operations.mu.Lock()
	if operations.guardExitPendingLocked() {
		operations.mu.Unlock()
		return rejected("guard_exit_pending"), nil
	}
	if operations.localAppData != "" {
		if err := appsettings.PauseRecovery(operations.localAppData, true); err != nil {
			operations.recordLocked("recovery_pause_failed")
		}
	}
	operations.recordLocked("safe_stop_requested")
	if operations.sessionDone != nil {
		operations.sessionState = "DISCONNECTING"
	}
	operations.mu.Unlock()
	operations.stopAgent(errors.New("local safe stop requested"))
	return completed("safe_stop_requested"), nil
}

func (operations *Operations) Close(ctx context.Context) error {
	if operations == nil {
		return nil
	}
	operations.stopAgent(context.Canceled)
	if workflow, ok := operations.pairing.(interface{ Close(context.Context) error }); ok {
		if err := workflow.Close(ctx); err != nil {
			return err
		}
	}
	result, err := operations.disconnect(ctx, false)
	if err != nil {
		return err
	}
	if result.Code == "no_active_session" || result.Status == "COMPLETED" {
		return nil
	}
	return errors.New(result.Code)
}

func (operations *Operations) finishSession(done chan struct{}, outcome *sessionOutcome, err error, cancelled, displayChanged bool) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if operations.sessionDone == done {
		var pending *crashguard.ExitPending
		if errors.As(err, &pending) {
			operations.guardExitDone = pending.Done
		}
	}
	displayChanged = displayChanged && outcome.connected && !operations.manualStop && operations.root.Err() == nil
	cancelled = cancelled || displayChanged
	outcome.failed = !cancelled || operations.sessionFaulted
	if operations.sessionDone == done {
		if outcome.failed && !cancelled {
			failure := agent.DescribeSessionFailure(err)
			operations.recordSessionFaultLocked(outcome, productlog.FaultCode(failure.Component, failure.Phase, failure.Code))
			if phase := agent.SupervisorExitPhase(err); phase != "" {
				operations.recordSessionFaultLocked(outcome, productlog.FaultCode("supervisor", phase, "failed"))
			}
		}
		operations.sessionStop = nil
		operations.sessionDone = nil
		operations.sessionOutcome = nil
		operations.sessionState = ""
		operations.inputBackend = "UNKNOWN"
		operations.boundaryBlocked, operations.boundaryBlockedAt = "", time.Time{}
		operations.sessionFaulted = operations.sessionFaulted || outcome.failed
		if displayChanged && !outcome.failed {
			operations.lastStopReason = "display_change"
		} else {
			operations.lastStopReason = ""
		}
		operations.peerAlias = ""
		if outcome.failed {
			operations.recordLocked("session_failed")
		} else {
			operations.recordLocked("session_stopped")
		}
	}
	// Publish completion only after clearing the session, and wake every waiter.
	close(done)
}

func (operations *Operations) recordLocked(code string) {
	operations.log.Event(code)
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
