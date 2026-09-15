package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
)

type SupervisorState string

const (
	SupervisorStopped   SupervisorState = "STOPPED"
	SupervisorAcquiring SupervisorState = "ACQUIRING"
	SupervisorActive    SupervisorState = "ACTIVE"
	SupervisorBackoff   SupervisorState = "BACKOFF"
)

type SupervisorEvent string

const (
	SupervisorStarted            SupervisorEvent = "STARTED"
	SupervisorConnectionAcquired SupervisorEvent = "CONNECTION_ACQUIRED"
	SupervisorAcquireFailed      SupervisorEvent = "ACQUIRE_FAILED"
	SupervisorSessionFailed      SupervisorEvent = "SESSION_FAILED"
	SupervisorRetryElapsed       SupervisorEvent = "RETRY_ELAPSED"
	SupervisorStoppedEvent       SupervisorEvent = "STOPPED"
)

type SupervisorTransition struct {
	Previous SupervisorState
	Event    SupervisorEvent
	Next     SupervisorState
}

func TransitionSupervisor(state SupervisorState, event SupervisorEvent) (SupervisorTransition, error) {
	next, allowed := supervisorTransitions[supervisorTransitionKey{state: state, event: event}]
	if !allowed {
		return SupervisorTransition{}, fmt.Errorf("invalid supervisor transition: state=%s event=%s", state, event)
	}
	return SupervisorTransition{Previous: state, Event: event, Next: next}, nil
}

type supervisorTransitionKey struct {
	state SupervisorState
	event SupervisorEvent
}

var supervisorTransitions = map[supervisorTransitionKey]SupervisorState{
	{SupervisorStopped, SupervisorStarted}:              SupervisorAcquiring,
	{SupervisorAcquiring, SupervisorConnectionAcquired}: SupervisorActive,
	{SupervisorAcquiring, SupervisorAcquireFailed}:      SupervisorBackoff,
	{SupervisorActive, SupervisorSessionFailed}:         SupervisorBackoff,
	{SupervisorBackoff, SupervisorRetryElapsed}:         SupervisorAcquiring,
	{SupervisorAcquiring, SupervisorStoppedEvent}:       SupervisorStopped,
	{SupervisorActive, SupervisorStoppedEvent}:          SupervisorStopped,
	{SupervisorBackoff, SupervisorStoppedEvent}:         SupervisorStopped,
}

type ConnectionSource interface {
	Next(context.Context) (Connection, error)
	Close() error
}

type SessionRunner func(context.Context, Config, Dependencies) (Result, error)

type SupervisorObserver interface {
	ObserveSupervisor(SupervisorSnapshot)
}

type SupervisorSnapshot struct {
	State            SupervisorState
	Attempt          uint32
	RetryAfter       time.Duration
	FailureKind      string
	FailureComponent string
	FailurePhase     string
	FailureCode      string
	// Independent of the primary descriptor: cleanup/read errors may coexist.
	PeerTimeoutObserved bool
}

type BackoffPolicy struct {
	Initial     time.Duration
	Maximum     time.Duration
	StableAfter time.Duration
}

func DefaultBackoffPolicy() BackoffPolicy {
	return BackoffPolicy{Initial: 250 * time.Millisecond, Maximum: 30 * time.Second, StableAfter: 30 * time.Second}
}

func (policy BackoffPolicy) Delay(attempt uint32) (time.Duration, error) {
	if err := policy.validate(); err != nil {
		return 0, err
	}
	if attempt == 0 {
		return 0, errors.New("backoff attempt must be positive")
	}
	delay := policy.Initial
	for current := uint32(1); current < attempt && delay < policy.Maximum; current++ {
		if delay > policy.Maximum/2 {
			delay = policy.Maximum
			break
		}
		delay *= 2
	}
	if delay > policy.Maximum {
		delay = policy.Maximum
	}
	return delay, nil
}

func (policy BackoffPolicy) validate() error {
	if policy.Initial <= 0 || policy.Maximum < policy.Initial || policy.StableAfter <= 0 {
		return errors.New("backoff initial, maximum, and stable-after values are invalid")
	}
	return nil
}

type SupervisorConfig struct {
	Session  Config
	Backoff  BackoffPolicy
	Now      func() time.Time
	Wait     func(context.Context, time.Duration) error
	Observer SupervisorObserver
}

type SupervisorDependencies struct {
	Source       ConnectionSource
	Hook         SourceHook
	Input        InputBoundaryPort
	Effects      application.EffectExecutor
	Authorizer   peer.LeaseAuthorizer
	Management   *SessionRegistry
	TrustRevoker TrustRevoker
	RunSession   SessionRunner
}

func RunSupervisor(ctx context.Context, config SupervisorConfig, dependencies SupervisorDependencies) (returnErr error) {
	if ctx == nil {
		return errors.New("supervisor context is required")
	}
	if err := validateSupervisor(config, dependencies); err != nil {
		return err
	}
	state := SupervisorStopped
	transition := func(event SupervisorEvent, snapshot SupervisorSnapshot) error {
		next, err := TransitionSupervisor(state, event)
		if err != nil {
			return err
		}
		state = next.Next
		snapshot.State = state
		if config.Observer != nil {
			config.Observer.ObserveSupervisor(snapshot)
		}
		return nil
	}
	if err := transition(SupervisorStarted, SupervisorSnapshot{}); err != nil {
		return errors.Join(err, dependencies.Source.Close())
	}
	terminalSnapshot := SupervisorSnapshot{}
	defer func() {
		sourceCloseErr := dependencies.Source.Close()
		var transitionErr error
		if state != SupervisorStopped {
			transitionErr = transition(SupervisorStoppedEvent, terminalSnapshot)
		}
		returnErr = errors.Join(returnErr, sourceCloseErr, transitionErr)
	}()

	var attempt uint32
	for {
		connection, acquireErr := dependencies.Source.Next(ctx)
		if acquireErr != nil {
			if connection != nil {
				if closeErr := connection.Close(); closeErr != nil {
					return errors.Join(acquireErr, ctx.Err(), closeErr)
				}
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			attempt++
			delay, err := config.Backoff.Delay(attempt)
			if err != nil {
				return err
			}
			if err := transition(SupervisorAcquireFailed, SupervisorSnapshot{Attempt: attempt, RetryAfter: delay, FailureKind: "acquire"}); err != nil {
				return err
			}
			if err := waitForRetry(ctx, config, attempt, delay, transition); err != nil {
				return err
			}
			continue
		}
		if connection == nil {
			return errors.New("connection source returned a nil connection")
		}
		ownedConnection := &supervisedConnection{Connection: connection}
		if err := transition(SupervisorConnectionAcquired, SupervisorSnapshot{Attempt: attempt}); err != nil {
			return errors.Join(err, ownedConnection.Close())
		}
		started := config.Now()
		result, sessionErr := dependencies.RunSession(ctx, config.Session, Dependencies{
			Connection: ownedConnection, Hook: dependencies.Hook, Input: dependencies.Input,
			Effects: dependencies.Effects, Authorizer: dependencies.Authorizer,
			Management: dependencies.Management, TrustRevoker: dependencies.TrustRevoker,
		})
		if closeErr := ownedConnection.Close(); closeErr != nil {
			if !errors.Is(sessionErr, closeErr) {
				return errors.Join(sessionErr, ctx.Err(), closeErr)
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if sessionErr == nil {
			return errors.New("agent session ended without a failure or shutdown")
		}
		if errors.Is(sessionErr, wininput.ErrEmergencyRelease) {
			terminalSnapshot.FailureKind = "emergency_release"
			return sessionErr
		}
		if result.FinalState != control.StateLocal {
			return fmt.Errorf("agent session cannot reconnect outside LOCAL: %w", sessionErr)
		}
		if ownedConnection.disconnectReceived.Load() || errors.Is(sessionErr, peer.ErrPeerDisconnectRequested) {
			terminalSnapshot.FailureKind = "peer_disconnect"
			return sessionErr
		}
		if config.Now().Sub(started) >= config.Backoff.StableAfter {
			attempt = 0
		}
		attempt++
		delay, err := config.Backoff.Delay(attempt)
		if err != nil {
			return err
		}
		failure := DescribeSessionFailure(sessionErr)
		if err := transition(SupervisorSessionFailed, SupervisorSnapshot{
			Attempt: attempt, RetryAfter: delay, FailureKind: "session",
			FailureComponent:    failure.Component,
			FailurePhase:        failure.Phase,
			FailureCode:         failure.Code,
			PeerTimeoutObserved: errors.Is(sessionErr, peer.ErrPeerLivenessExpired),
		}); err != nil {
			return err
		}
		if err := waitForRetry(ctx, config, attempt, delay, transition); err != nil {
			return err
		}
	}
}

func waitForRetry(ctx context.Context, config SupervisorConfig, attempt uint32, delay time.Duration, transition func(SupervisorEvent, SupervisorSnapshot) error) error {
	if err := config.Wait(ctx, delay); err != nil {
		return err
	}
	return transition(SupervisorRetryElapsed, SupervisorSnapshot{Attempt: attempt})
}

type supervisedConnection struct {
	Connection
	closeOnce          sync.Once
	closeErr           error
	disconnectReceived atomic.Bool
}

func (connection *supervisedConnection) ReadFrame() (transport.Frame, error) {
	frame, err := connection.Connection.ReadFrame()
	if err == nil && frame.Kind == transport.MessageDisconnect && len(frame.Payload) == 0 {
		connection.disconnectReceived.Store(true)
	}
	return frame, err
}

func (connection *supervisedConnection) Close() error {
	connection.closeOnce.Do(func() {
		connection.closeErr = connection.Connection.Close()
	})
	return connection.closeErr
}

func validateSupervisor(config SupervisorConfig, dependencies SupervisorDependencies) error {
	if err := config.Backoff.validate(); err != nil {
		return err
	}
	if config.Now == nil || config.Wait == nil {
		return errors.New("supervisor clock and wait function are required")
	}
	if dependencies.Source == nil || dependencies.Hook == nil || dependencies.Input == nil || dependencies.Effects == nil || dependencies.Authorizer == nil || dependencies.RunSession == nil {
		return errors.New("supervisor source, agent ports, and session runner are required")
	}
	return nil
}

func Wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
