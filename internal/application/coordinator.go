package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

var ErrControlEffectsFailed = errors.New("control transition effects failed")

type EffectExecutor interface {
	Execute(context.Context, control.Effect) error
}

type EffectFailure struct {
	Effect control.Effect
	Err    error
}

func (failure EffectFailure) Error() string {
	return fmt.Sprintf("%s failed: %v", failure.Effect, failure.Err)
}

func (failure EffectFailure) Unwrap() error {
	return failure.Err
}

type ControlOutcome struct {
	Snapshot    control.Snapshot
	Transitions []control.Transition
	Attempted   []control.Effect
}

func HandleControlEvent(
	ctx context.Context,
	current control.Snapshot,
	event control.Event,
	executor EffectExecutor,
) (ControlOutcome, error) {
	if executor == nil {
		return ControlOutcome{}, errors.New("effect executor is required")
	}
	decision, err := control.Apply(current, event)
	if err != nil {
		return ControlOutcome{}, err
	}
	outcome := ControlOutcome{
		Snapshot:    decision.Next,
		Transitions: []control.Transition{decision},
	}
	failures := executeEffects(ctx, executor, decision.Effects, &outcome)
	if len(failures) == 0 {
		completionFailures := completeSuccessfulRelease(ctx, executor, decision, &outcome)
		if len(completionFailures) == 0 {
			return outcome, nil
		}
		return outcome, joinEffectFailures(completionFailures)
	}

	fallbackBase := decision.Next
	fallbackEvent := control.Event{Kind: control.EventFaultDetected}
	if decision.Next.State == control.StateLocal {
		fallbackBase = decision.Previous
		fallbackEvent = control.Event{Kind: control.EventReleaseAllFailed}
	} else if decision.Next.State == control.StateFault {
		return outcome, joinEffectFailures(failures)
	}

	fallback, fallbackErr := control.Apply(fallbackBase, fallbackEvent)
	if fallbackErr != nil {
		return outcome, errors.Join(joinEffectFailures(failures), fallbackErr)
	}
	outcome.Snapshot = fallback.Next
	outcome.Transitions = append(outcome.Transitions, fallback)
	fallbackFailures := executeEffects(ctx, executor, fallback.Effects, &outcome)
	failures = append(failures, fallbackFailures...)
	if len(fallbackFailures) == 0 && decision.Next.State != control.StateLocal {
		failures = append(failures, completeSuccessfulRelease(ctx, executor, fallback, &outcome)...)
	}
	return outcome, joinEffectFailures(failures)
}

func HandleLiveness(
	ctx context.Context,
	current control.Snapshot,
	liveness control.PeerLiveness,
	now time.Time,
	timeout time.Duration,
	executor EffectExecutor,
) (ControlOutcome, bool, error) {
	if current.State == control.StateLocal || current.SessionID == "" {
		return ControlOutcome{}, false, errors.New("liveness checks require a non-local control session")
	}
	if current.SessionID != liveness.SessionID {
		mismatch := fmt.Errorf("%w: liveness session does not match control session", control.ErrStaleHeartbeat)
		outcome, faultErr := HandleControlEvent(ctx, current, control.Event{Kind: control.EventFaultDetected}, executor)
		return outcome, true, errors.Join(mismatch, faultErr)
	}
	event, expired, err := liveness.FaultEventIfExpired(now, timeout)
	if err != nil {
		outcome, faultErr := HandleControlEvent(ctx, current, control.Event{Kind: control.EventFaultDetected}, executor)
		return outcome, true, errors.Join(err, faultErr)
	}
	if !expired {
		return ControlOutcome{Snapshot: current}, false, nil
	}
	outcome, err := HandleControlEvent(ctx, current, event, executor)
	return outcome, true, err
}

func HandleInputEnvelope(
	ctx context.Context,
	current control.Snapshot,
	stream control.InputStream,
	envelope control.InputEnvelope,
	executor EffectExecutor,
) (control.InputStream, ControlOutcome, bool, error) {
	if current.State != control.StateInboundActive {
		directionErr := errors.New("input envelopes require an inbound-active lease")
		if current.State == control.StateLocal {
			return control.InputStream{}, ControlOutcome{Snapshot: current}, false, directionErr
		}
		outcome, faultErr := HandleControlEvent(ctx, current, control.Event{Kind: control.EventFaultDetected}, executor)
		return control.InputStream{}, outcome, false, errors.Join(directionErr, faultErr)
	}
	if current.SessionID != stream.SessionID {
		mismatch := fmt.Errorf("%w: input stream session does not match control session", control.ErrStaleSession)
		outcome, faultErr := HandleControlEvent(ctx, current, control.Event{Kind: control.EventFaultDetected}, executor)
		return control.InputStream{}, outcome, false, errors.Join(mismatch, faultErr)
	}
	next, err := stream.Accept(envelope)
	if err == nil {
		return next, ControlOutcome{Snapshot: current}, true, nil
	}
	outcome, faultErr := HandleControlEvent(ctx, current, control.Event{Kind: control.EventFaultDetected}, executor)
	return control.InputStream{}, outcome, false, errors.Join(err, faultErr)
}

func executeEffects(
	ctx context.Context,
	executor EffectExecutor,
	effects []control.Effect,
	outcome *ControlOutcome,
) []error {
	failures := make([]error, 0)
	for _, effect := range effects {
		outcome.Attempted = append(outcome.Attempted, effect)
		if err := executor.Execute(ctx, effect); err != nil {
			failures = append(failures, EffectFailure{Effect: effect, Err: err})
		}
	}
	return failures
}

func joinEffectFailures(failures []error) error {
	return errors.Join(append([]error{ErrControlEffectsFailed}, failures...)...)
}

func completeSuccessfulRelease(
	ctx context.Context,
	executor EffectExecutor,
	decision control.Transition,
	outcome *ControlOutcome,
) []error {
	if !containsEffect(decision.Effects, control.EffectReleaseAll) ||
		(decision.Next.State != control.StateRevoking && decision.Next.State != control.StateFault) {
		return nil
	}
	completion, err := control.Apply(decision.Next, control.Event{Kind: control.EventReleaseAllSucceeded})
	if err != nil {
		return []error{err}
	}
	outcome.Snapshot = completion.Next
	outcome.Transitions = append(outcome.Transitions, completion)
	failures := executeEffects(ctx, executor, completion.Effects, outcome)
	if len(failures) == 0 {
		return nil
	}

	rollback, rollbackErr := control.Apply(completion.Previous, control.Event{Kind: control.EventReleaseAllFailed})
	if rollbackErr != nil {
		return append(failures, rollbackErr)
	}
	outcome.Snapshot = rollback.Next
	outcome.Transitions = append(outcome.Transitions, rollback)
	return append(failures, executeEffects(ctx, executor, rollback.Effects, outcome)...)
}

func containsEffect(effects []control.Effect, target control.Effect) bool {
	for _, effect := range effects {
		if effect == target {
			return true
		}
	}
	return false
}
