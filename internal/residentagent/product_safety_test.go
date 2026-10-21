package residentagent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/wininput"
)

type safetyInput struct {
	agent.InputBoundaryPort
	events               *[]string
	pressErr, releaseErr error
}

func (input safetyInput) Press(context.Context, control.PressedToken) error {
	*input.events = append(*input.events, "inject_press")
	return input.pressErr
}
func (input safetyInput) Release(context.Context, control.PressedToken) error {
	*input.events = append(*input.events, "inject_release")
	return input.releaseErr
}

type safetyGuard struct{ events *[]string }

func (guard safetyGuard) PreparePressContext(context.Context, control.PressedToken) (bool, error) {
	*guard.events = append(*guard.events, "prepare")
	return true, nil
}
func (guard safetyGuard) CancelPreparedPress(control.PressedToken) error {
	*guard.events = append(*guard.events, "cancel_prepare")
	return nil
}
func (guard safetyGuard) Released(control.PressedToken) error {
	*guard.events = append(*guard.events, "released")
	return nil
}

func TestProductGuardRetainsUncertainInjectionObligations(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure error
		want    []string
	}{
		{"success", nil, []string{"prepare", "inject_press"}},
		{"not_applied", &application.InjectionError{Certainty: application.InjectionNotApplied}, []string{"prepare", "inject_press", "cancel_prepare"}},
		{"partial", &application.InjectionError{Certainty: application.InjectionPartiallyApplied}, []string{"prepare", "inject_press"}},
		{"unknown", errors.New("unknown injection"), []string{"prepare", "inject_press"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var events []string
			input := &guardedProductInput{InputBoundaryPort: safetyInput{events: &events, pressErr: test.failure}, guard: safetyGuard{&events}}
			err := input.Press(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30})
			if (err != nil) != (test.failure != nil) || !reflect.DeepEqual(events, test.want) {
				t.Fatalf("events=%v error=%v", events, err)
			}
		})
	}
	var events []string
	input := &guardedProductInput{InputBoundaryPort: safetyInput{events: &events}, guard: safetyGuard{&events}}
	if err := input.Release(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"inject_release", "released"}) {
		t.Fatalf("release ordering=%v", events)
	}
}

func TestProductGuardRejectsProtectedCredentialInputBeforeInjection(t *testing.T) {
	var events []string
	input := &guardedProductInput{
		InputBoundaryPort: safetyInput{events: &events},
		guard:             safetyGuard{&events},
		protected:         func(context.Context) (bool, error) { return true, nil },
		stopProtected:     func() { events = append(events, "stop") },
	}
	if err := input.Press(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}); !errors.Is(err, wininput.ErrProtectedInput) {
		t.Fatalf("Press() error = %v", err)
	}
	if !reflect.DeepEqual(events, []string{"stop"}) {
		t.Fatalf("protected input touched the injection boundary: %v", events)
	}
}

func TestCancellationDoesNotHideCleanupFailure(t *testing.T) {
	if !cancellationOnly(context.Canceled, context.Canceled) {
		t.Fatal("expected cancellation rejected")
	}
	for _, err := range []error{errors.Join(context.Canceled, errors.New("cleanup failed")), &application.InjectionError{Certainty: application.InjectionOutcomeUnknown}} {
		if cancellationOnly(err, context.Canceled) {
			t.Fatalf("cleanup failure suppressed: %v", err)
		}
	}
}

func TestProductGuardExpiredWaitDoesNotInjectOrClearObligation(t *testing.T) {
	for _, press := range []bool{true, false} {
		var events []string
		input := &guardedProductInput{InputBoundaryPort: safetyInput{events: &events}, guard: safetyGuard{&events}}
		input.mu.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		done := make(chan error, 1)
		go func() {
			token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
			if press {
				done <- input.Press(ctx, token)
			} else {
				done <- input.Release(ctx, token)
			}
		}()
		select {
		case err := <-done:
			input.mu.Unlock()
			cancel()
			certainty, ok := application.InjectionCertaintyOf(err)
			if !errors.Is(err, context.DeadlineExceeded) || !ok || certainty != application.InjectionNotApplied || len(events) != 0 {
				t.Fatal("expired waiter mutated native or guard state", err, events)
			}
		case <-time.After(time.Second):
			input.mu.Unlock()
			cancel()
			t.Fatal("guard admission exceeded caller budget")
		}
		if err := input.Release(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(events, []string{"inject_release", "released"}) {
			t.Fatal("fresh cleanup lost its obligation", events)
		}
	}
}
