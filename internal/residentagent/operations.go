package residentagent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
)

const DefaultSessionStopTimeout = 5 * time.Second

var (
	ErrNotConfigured = errors.New("resident agent operation is not configured")
	ErrAlreadyActive = errors.New("resident agent session is already active")
)

type SessionRunner func(context.Context, string) error

type PairWorkflow interface {
	Handle(context.Context, localapi.OperationRequest) (localapi.OperationResult, error)
}

type Operations struct {
	root      context.Context
	stopAgent context.CancelCauseFunc
	identity  productidentity.PublicIdentity
	runner    SessionRunner
	pairing   PairWorkflow

	mu          sync.Mutex
	sessionStop context.CancelCauseFunc
	sessionDone chan error
	peerAlias   string
	lastCode    string
	counters    map[string]uint64
}

func New(parent context.Context, identity productidentity.PublicIdentity, runner SessionRunner, pairing PairWorkflow) (*Operations, error) {
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
		lastCode:  "ready",
		counters:  map[string]uint64{},
	}, nil
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
	done := make(chan error, 1)
	operations.sessionStop = cancel
	operations.sessionDone = done
	operations.peerAlias = request.PeerAlias
	operations.recordLocked("connect_started")
	go func(alias string) {
		done <- operations.runner(ctx, alias)
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

func (operations *Operations) finishSession(done chan error) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if operations.sessionDone == done {
		operations.sessionStop = nil
		operations.sessionDone = nil
		operations.peerAlias = ""
		operations.recordLocked("session_stopped")
	}
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
