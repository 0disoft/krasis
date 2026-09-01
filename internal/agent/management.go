package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/transport"
)

const DefaultManagementRequestCapacity = 64

type ManagementCapability string

const ManagementCapabilityRevokeLease ManagementCapability = "LEASE_REVOKE"

type LeaseRevocationAction string

const (
	LeaseRevocationOnly       LeaseRevocationAction = "REVOKE_LEASE"
	LeaseRevocationDisconnect LeaseRevocationAction = "DISCONNECT_PEER"
	LeaseRevocationTrust      LeaseRevocationAction = "REVOKE_TRUST"
)

type LeaseRevocationStatus string

const (
	LeaseRevocationCompleted LeaseRevocationStatus = "COMPLETED"
	LeaseRevocationRejected  LeaseRevocationStatus = "REJECTED"
	LeaseRevocationFailed    LeaseRevocationStatus = "FAILED"
)

const (
	ManagementCodeCompleted        = "completed"
	ManagementCodeUnauthenticated  = "unauthenticated"
	ManagementCodeForbidden        = "forbidden"
	ManagementCodeInvalidRequest   = "invalid_request"
	ManagementCodeRequestConflict  = "request_conflict"
	ManagementCodeRequestCapacity  = "request_capacity_exhausted"
	ManagementCodeNoActiveSession  = "no_active_session"
	ManagementCodePeerMismatch     = "peer_mismatch"
	ManagementCodeLeaseMismatch    = "lease_mismatch"
	ManagementCodeTrustRevocation  = "trust_revocation_failed"
	ManagementCodeLocalCleanup     = "local_cleanup_failed"
	ManagementCodePeerNotification = "peer_notification_failed"
	ManagementCodeDisconnect       = "disconnect_failed"
	ManagementCodeRequestCanceled  = "request_canceled"
	ManagementCodeRequestDeadline  = "request_deadline_exceeded"
	ManagementCodeInternal         = "internal_failure"
)

var (
	ErrManagementUnauthenticated  = errors.New("management caller is not authenticated")
	ErrManagementForbidden        = errors.New("management caller cannot revoke leases")
	ErrManagementInvalidRequest   = errors.New("management request is invalid")
	ErrManagementRequestConflict  = errors.New("management request identifier was reused with different content")
	ErrManagementRequestCapacity  = errors.New("management request capacity is exhausted")
	ErrManagementNoActiveSession  = errors.New("no managed peer session is active")
	ErrManagementPeerMismatch     = errors.New("management peer identity does not match the active session")
	ErrManagementLeaseMismatch    = errors.New("management lease identity does not match the active lease")
	ErrManagementTrustRevocation  = errors.New("management trust revocation failed")
	ErrManagementLocalCleanup     = errors.New("management local lease cleanup failed")
	ErrManagementPeerNotification = errors.New("management peer notification failed")
	ErrManagementDisconnect       = errors.New("management peer disconnect failed")
)

type ManagementPrincipal struct {
	subject        string
	canRevokeLease bool
}

func NewManagementPrincipal(subject string, capabilities ...ManagementCapability) (ManagementPrincipal, error) {
	if err := validateManagementValue("management subject", subject, 128); err != nil {
		return ManagementPrincipal{}, err
	}
	principal := ManagementPrincipal{subject: subject}
	for _, capability := range capabilities {
		if capability != ManagementCapabilityRevokeLease {
			return ManagementPrincipal{}, fmt.Errorf("unsupported management capability %q", capability)
		}
		principal.canRevokeLease = true
	}
	return principal, nil
}

func (principal ManagementPrincipal) Subject() string { return principal.subject }

func (principal ManagementPrincipal) authorizeLeaseRevocation() error {
	if principal.subject == "" {
		return ErrManagementUnauthenticated
	}
	if !principal.canRevokeLease {
		return ErrManagementForbidden
	}
	return nil
}

type LeaseRevocationRequest struct {
	RequestID string
	Action    LeaseRevocationAction
	Peer      transport.AuthenticatedPeerContext
	Lease     application.LeaseIdentity
}

func (request LeaseRevocationRequest) Validate() error {
	if err := validateManagementValue("request identifier", request.RequestID, 128); err != nil {
		return errors.Join(ErrManagementInvalidRequest, err)
	}
	if request.Action != LeaseRevocationOnly &&
		request.Action != LeaseRevocationDisconnect &&
		request.Action != LeaseRevocationTrust {
		return errors.Join(ErrManagementInvalidRequest, errors.New("revocation action is unsupported"))
	}
	if err := request.Peer.Validate(); err != nil {
		return errors.Join(ErrManagementInvalidRequest, err)
	}
	for name, value := range map[string]string{
		"session identifier": request.Lease.SessionID,
		"lease identifier":   request.Lease.LeaseID,
		"lease epoch":        request.Lease.Epoch,
	} {
		if err := validateManagementValue(name, value, 256); err != nil {
			return errors.Join(ErrManagementInvalidRequest, err)
		}
	}
	return nil
}

func validateManagementValue(name, value string, maximum int) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > maximum {
		return fmt.Errorf("%s exceeds its size limit", name)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s cannot have surrounding whitespace", name)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s contains a control character", name)
		}
	}
	return nil
}

type LeaseRevocationResult struct {
	Status   LeaseRevocationStatus `json:"status"`
	Code     string                `json:"code"`
	Replayed bool                  `json:"replayed"`
}

type ManagementError struct {
	code  string
	cause error
}

func (failure *ManagementError) Error() string {
	if failure == nil || failure.code == "" {
		return "management request failed"
	}
	return "management request failed: " + failure.code
}

func (failure *ManagementError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

func (failure *ManagementError) Code() string {
	if failure == nil {
		return ""
	}
	return failure.code
}

type ManagementMetrics struct {
	Outcomes map[string]uint64 `json:"outcomes"`
	Replayed uint64            `json:"replayed"`
}

type revocableSession interface {
	Revoke(context.Context, LeaseRevocationRequest) error
}

type managementRequestRecord struct {
	request LeaseRevocationRequest
	done    chan struct{}
	result  LeaseRevocationResult
}

type registeredSession struct {
	generation uint64
	session    revocableSession
}

type SessionRegistry struct {
	mu              sync.Mutex
	active          *registeredSession
	nextGeneration  uint64
	requests        map[string]*managementRequestRecord
	completedOrder  []string
	requestCapacity int
	outcomes        map[string]uint64
	replayed        uint64
}

func NewSessionRegistry(requestCapacity int) (*SessionRegistry, error) {
	if requestCapacity <= 0 {
		return nil, errors.New("management request capacity must be positive")
	}
	return &SessionRegistry{
		requests:        make(map[string]*managementRequestRecord, requestCapacity),
		requestCapacity: requestCapacity,
		outcomes:        make(map[string]uint64),
	}, nil
}

func NewDefaultSessionRegistry() *SessionRegistry {
	registry, _ := NewSessionRegistry(DefaultManagementRequestCapacity)
	return registry
}

func (registry *SessionRegistry) Metrics() ManagementMetrics {
	if registry == nil {
		return ManagementMetrics{Outcomes: map[string]uint64{}}
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	outcomes := make(map[string]uint64, len(registry.outcomes))
	for code, count := range registry.outcomes {
		outcomes[code] = count
	}
	return ManagementMetrics{Outcomes: outcomes, Replayed: registry.replayed}
}

func (registry *SessionRegistry) Revoke(
	ctx context.Context,
	principal ManagementPrincipal,
	request LeaseRevocationRequest,
) (LeaseRevocationResult, error) {
	if registry == nil || registry.requests == nil || registry.outcomes == nil || registry.requestCapacity <= 0 {
		return LeaseRevocationResult{}, errors.New("management registry is not initialized")
	}
	if ctx == nil {
		return registry.finishImmediate(ErrManagementInvalidRequest)
	}
	if err := principal.authorizeLeaseRevocation(); err != nil {
		return registry.finishImmediate(err)
	}
	if err := request.Validate(); err != nil {
		return registry.finishImmediate(err)
	}

	record, session, owner, err := registry.beginRequest(request)
	if err != nil {
		return registry.finishImmediate(err)
	}
	if !owner {
		select {
		case <-ctx.Done():
			registry.recordReplay()
			result, waitErr := registry.finishImmediate(ctx.Err())
			result.Replayed = true
			return result, waitErr
		case <-record.done:
			registry.recordReplay()
			result := record.result
			result.Replayed = true
			return result, resultError(result)
		}
	}

	if session == nil {
		err = ErrManagementNoActiveSession
	} else {
		err = session.Revoke(ctx, request)
	}
	result := resultForManagementError(err)
	registry.completeRequest(request.RequestID, record, result)
	registry.recordOutcome(result.Code)
	return result, managementError(result.Code, err)
}

func (registry *SessionRegistry) finishImmediate(err error) (LeaseRevocationResult, error) {
	result := resultForManagementError(err)
	registry.recordOutcome(result.Code)
	return result, managementError(result.Code, err)
}

func (registry *SessionRegistry) recordOutcome(code string) {
	registry.mu.Lock()
	registry.outcomes[code]++
	registry.mu.Unlock()
}

func (registry *SessionRegistry) recordReplay() {
	registry.mu.Lock()
	registry.replayed++
	registry.mu.Unlock()
}

func (registry *SessionRegistry) beginRequest(request LeaseRevocationRequest) (
	*managementRequestRecord,
	revocableSession,
	bool,
	error,
) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if existing := registry.requests[request.RequestID]; existing != nil {
		if existing.request != request {
			return nil, nil, false, ErrManagementRequestConflict
		}
		return existing, nil, false, nil
	}
	registry.pruneCompletedLocked()
	if len(registry.requests) >= registry.requestCapacity {
		return nil, nil, false, ErrManagementRequestCapacity
	}
	record := &managementRequestRecord{request: request, done: make(chan struct{})}
	registry.requests[request.RequestID] = record
	if registry.active == nil {
		return record, nil, true, nil
	}
	return record, registry.active.session, true, nil
}

func (registry *SessionRegistry) completeRequest(
	requestID string,
	record *managementRequestRecord,
	result LeaseRevocationResult,
) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.requests[requestID] != record || record == nil {
		return
	}
	record.result = result
	registry.completedOrder = append(registry.completedOrder, requestID)
	close(record.done)
}

func (registry *SessionRegistry) pruneCompletedLocked() {
	for len(registry.requests) >= registry.requestCapacity && len(registry.completedOrder) > 0 {
		requestID := registry.completedOrder[0]
		registry.completedOrder = registry.completedOrder[1:]
		record := registry.requests[requestID]
		if record == nil {
			continue
		}
		select {
		case <-record.done:
			delete(registry.requests, requestID)
		default:
		}
	}
}

type sessionRegistration struct {
	registry   *SessionRegistry
	generation uint64
	once       sync.Once
}

func (registry *SessionRegistry) register(session revocableSession) (*sessionRegistration, error) {
	if registry == nil || registry.requests == nil || session == nil {
		return nil, errors.New("management registry and revocable session are required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.active != nil {
		return nil, errors.New("management registry already has an active session")
	}
	registry.nextGeneration++
	registry.active = &registeredSession{generation: registry.nextGeneration, session: session}
	return &sessionRegistration{registry: registry, generation: registry.nextGeneration}, nil
}

func (registration *sessionRegistration) Close() {
	if registration == nil || registration.registry == nil {
		return
	}
	registration.once.Do(func() { registration.registry.unregister(registration.generation) })
}

func (registry *SessionRegistry) unregister(generation uint64) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.active != nil && registry.active.generation == generation {
		registry.active = nil
	}
}

func resultForManagementError(err error) LeaseRevocationResult {
	code := codeForManagementError(err)
	status := LeaseRevocationFailed
	if code == ManagementCodeCompleted {
		status = LeaseRevocationCompleted
	} else if code == ManagementCodeUnauthenticated ||
		code == ManagementCodeForbidden ||
		code == ManagementCodeInvalidRequest ||
		code == ManagementCodeRequestConflict ||
		code == ManagementCodeRequestCapacity ||
		code == ManagementCodeNoActiveSession ||
		code == ManagementCodePeerMismatch ||
		code == ManagementCodeLeaseMismatch {
		status = LeaseRevocationRejected
	}
	return LeaseRevocationResult{Status: status, Code: code}
}

func codeForManagementError(err error) string {
	switch {
	case err == nil:
		return ManagementCodeCompleted
	case errors.Is(err, ErrManagementUnauthenticated):
		return ManagementCodeUnauthenticated
	case errors.Is(err, ErrManagementForbidden):
		return ManagementCodeForbidden
	case errors.Is(err, ErrManagementInvalidRequest):
		return ManagementCodeInvalidRequest
	case errors.Is(err, ErrManagementRequestConflict):
		return ManagementCodeRequestConflict
	case errors.Is(err, ErrManagementRequestCapacity):
		return ManagementCodeRequestCapacity
	case errors.Is(err, ErrManagementNoActiveSession):
		return ManagementCodeNoActiveSession
	case errors.Is(err, ErrManagementPeerMismatch):
		return ManagementCodePeerMismatch
	case errors.Is(err, ErrManagementLeaseMismatch):
		return ManagementCodeLeaseMismatch
	case errors.Is(err, ErrManagementLocalCleanup):
		return ManagementCodeLocalCleanup
	case errors.Is(err, ErrManagementPeerNotification):
		return ManagementCodePeerNotification
	case errors.Is(err, ErrManagementDisconnect):
		return ManagementCodeDisconnect
	case errors.Is(err, ErrManagementTrustRevocation):
		return ManagementCodeTrustRevocation
	case errors.Is(err, context.Canceled):
		return ManagementCodeRequestCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return ManagementCodeRequestDeadline
	default:
		return ManagementCodeInternal
	}
}

func managementError(code string, cause error) error {
	if code == ManagementCodeCompleted {
		return nil
	}
	return &ManagementError{code: code, cause: cause}
}

func resultError(result LeaseRevocationResult) error {
	return managementError(result.Code, nil)
}
