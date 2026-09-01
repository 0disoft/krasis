package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/transport"
)

func TestSessionRegistryRequiresAuthenticatedAuthorizedCaller(t *testing.T) {
	registry := NewDefaultSessionRegistry()
	request := managementTestRequest("request-auth", LeaseRevocationOnly)

	result, err := registry.Revoke(context.Background(), ManagementPrincipal{}, request)
	if result.Code != ManagementCodeUnauthenticated || !errors.Is(err, ErrManagementUnauthenticated) {
		t.Fatalf("unauthenticated result = %+v, error = %v", result, err)
	}

	principal, err := NewManagementPrincipal("local-client")
	if err != nil {
		t.Fatal(err)
	}
	result, err = registry.Revoke(context.Background(), principal, request)
	if result.Code != ManagementCodeForbidden || !errors.Is(err, ErrManagementForbidden) {
		t.Fatalf("forbidden result = %+v, error = %v", result, err)
	}

	metrics := registry.Metrics()
	if metrics.Outcomes[ManagementCodeUnauthenticated] != 1 ||
		metrics.Outcomes[ManagementCodeForbidden] != 1 {
		t.Fatalf("metrics = %+v", metrics)
	}
}

func TestSessionRegistryDeduplicatesConcurrentRequest(t *testing.T) {
	registry := NewDefaultSessionRegistry()
	started := make(chan struct{})
	release := make(chan struct{})
	session := &managementTestRevocableSession{started: started, release: release}
	registration, err := registry.register(session)
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Close()
	principal := managementTestPrincipal(t)
	request := managementTestRequest("request-dedupe", LeaseRevocationOnly)
	type outcome struct {
		result LeaseRevocationResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	go func() {
		result, revokeErr := registry.Revoke(context.Background(), principal, request)
		outcomes <- outcome{result: result, err: revokeErr}
	}()
	<-started
	go func() {
		result, revokeErr := registry.Revoke(context.Background(), principal, request)
		outcomes <- outcome{result: result, err: revokeErr}
	}()
	close(release)
	first := <-outcomes
	second := <-outcomes
	if first.err != nil || second.err != nil {
		t.Fatalf("errors = %v, %v", first.err, second.err)
	}
	if session.Calls() != 1 {
		t.Fatalf("session revoke calls = %d", session.Calls())
	}
	if first.result.Replayed == second.result.Replayed {
		t.Fatalf("replay flags = %v, %v", first.result.Replayed, second.result.Replayed)
	}
	if metrics := registry.Metrics(); metrics.Outcomes[ManagementCodeCompleted] != 1 || metrics.Replayed != 1 {
		t.Fatalf("metrics = %+v", metrics)
	}
}

func TestSessionRegistryRejectsRequestIDConflictAndBoundsInflightWork(t *testing.T) {
	registry, err := NewSessionRegistry(1)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	session := &managementTestRevocableSession{started: started, release: release}
	registration, err := registry.register(session)
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Close()
	principal := managementTestPrincipal(t)
	first := managementTestRequest("request-one", LeaseRevocationOnly)
	done := make(chan error, 1)
	go func() {
		_, revokeErr := registry.Revoke(context.Background(), principal, first)
		done <- revokeErr
	}()
	<-started

	conflict := first
	conflict.Action = LeaseRevocationDisconnect
	result, err := registry.Revoke(context.Background(), principal, conflict)
	if result.Code != ManagementCodeRequestConflict || !errors.Is(err, ErrManagementRequestConflict) {
		t.Fatalf("conflict result = %+v, error = %v", result, err)
	}

	second := managementTestRequest("request-two", LeaseRevocationOnly)
	result, err = registry.Revoke(context.Background(), principal, second)
	if result.Code != ManagementCodeRequestCapacity || !errors.Is(err, ErrManagementRequestCapacity) {
		t.Fatalf("capacity result = %+v, error = %v", result, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSessionRegistryGenerationCannotUnregisterReconnect(t *testing.T) {
	registry := NewDefaultSessionRegistry()
	oldRegistration, err := registry.register(&managementTestRevocableSession{})
	if err != nil {
		t.Fatal(err)
	}
	oldGeneration := oldRegistration.generation
	oldRegistration.Close()

	current := &managementTestRevocableSession{}
	currentRegistration, err := registry.register(current)
	if err != nil {
		t.Fatal(err)
	}
	defer currentRegistration.Close()
	registry.unregister(oldGeneration)

	result, err := registry.Revoke(
		context.Background(),
		managementTestPrincipal(t),
		managementTestRequest("request-current", LeaseRevocationOnly),
	)
	if err != nil || result.Code != ManagementCodeCompleted {
		t.Fatalf("current result = %+v, error = %v", result, err)
	}
	if current.Calls() != 1 {
		t.Fatalf("current session revoke calls = %d", current.Calls())
	}
}

func managementTestPrincipal(t *testing.T) ManagementPrincipal {
	t.Helper()
	principal, err := NewManagementPrincipal("local-client", ManagementCapabilityRevokeLease)
	if err != nil {
		t.Fatal(err)
	}
	return principal
}

func managementTestPeer() transport.AuthenticatedPeerContext {
	return transport.AuthenticatedPeerContext{
		CertificateSHA256: strings.Repeat("a", 64),
		Role:              transport.AuthenticatedPeerConnector,
	}
}

func managementTestRequest(requestID string, action LeaseRevocationAction) LeaseRevocationRequest {
	return LeaseRevocationRequest{
		RequestID: requestID,
		Action:    action,
		Peer:      managementTestPeer(),
		Lease: application.LeaseIdentity{
			SessionID: "session-1",
			LeaseID:   "lease-1",
			Epoch:     "epoch-1",
		},
	}
}

type managementTestRevocableSession struct {
	mu        sync.Mutex
	calls     int
	started   chan struct{}
	release   chan struct{}
	err       error
	startOnce sync.Once
}

func (session *managementTestRevocableSession) Revoke(context.Context, LeaseRevocationRequest) error {
	session.mu.Lock()
	session.calls++
	session.mu.Unlock()
	if session.started != nil {
		session.startOnce.Do(func() { close(session.started) })
	}
	if session.release != nil {
		<-session.release
	}
	return session.err
}

func (session *managementTestRevocableSession) Calls() int {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.calls
}
