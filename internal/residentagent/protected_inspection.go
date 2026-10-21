package residentagent

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/syncgate"
)

var errProtectedInspectionUnavailable = errors.New("protected input inspection unavailable")

// Expired native queries still own their slot until they really return. Each
// runner serializes its own inspections; replacements share this process cap.
var protectedInspectionSlots = make(chan struct{}, 4)

type protectedInspectionExpiry struct{ cause error }

func (*protectedInspectionExpiry) Error() string   { return "protected input inspection expired" }
func (e *protectedInspectionExpiry) Unwrap() error { return e.cause }
func (*protectedInspectionExpiry) Is(target error) bool {
	return target == errProtectedInspectionUnavailable
}

// A raw deadline from release/cleanup is not proof of this read-only timeout.
func protectedInspectionExpiredOnly(err error) bool {
	var expiry *protectedInspectionExpiry
	if !errors.As(err, &expiry) {
		return false
	}
	var only func(error) bool
	only = func(err error) bool {
		if expiry, ok := err.(*protectedInspectionExpiry); ok {
			return expiry.cause == context.DeadlineExceeded
		}
		if injection, ok := err.(*application.InjectionError); ok && injection.Certainty != application.InjectionNotApplied {
			return false
		}
		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			if len(joined.Unwrap()) == 0 {
				return false
			}
			for _, child := range joined.Unwrap() {
				if !only(child) {
					return false
				}
			}
			return true
		}
		if wrapped, ok := err.(interface{ Unwrap() error }); ok && wrapped.Unwrap() != nil {
			return only(wrapped.Unwrap())
		}
		return err == context.Canceled
	}
	return only(err)
}

func boundedProtectedInputInspection(inspect func() bool) func(context.Context) (bool, error) {
	var gate syncgate.Mutex
	return func(ctx context.Context) (bool, error) {
		if ctx == nil || inspect == nil {
			return false, errProtectedInspectionUnavailable
		}
		ctx, cancel := context.WithTimeout(ctx, application.InputInjectionTimeout)
		defer cancel()
		if err := gate.LockContext(ctx); err != nil {
			return false, &protectedInspectionExpiry{cause: err}
		}
		select {
		case protectedInspectionSlots <- struct{}{}:
		default:
			gate.Unlock()
			return false, errProtectedInspectionUnavailable
		}
		type observation struct {
			active bool
			err    error
		}
		done := make(chan observation, 1)
		go func() {
			result := observation{err: errProtectedInspectionUnavailable}
			defer func() {
				_ = recover() // Native/observer panic payloads are never exposed.
				done <- result
				<-protectedInspectionSlots
				gate.Unlock()
			}()
			if err := ctx.Err(); err != nil {
				result.err = err
				return
			}
			result.active = inspect() // Read-only; this task owns its native handles.
			result.err = nil
		}()
		select {
		case <-ctx.Done():
			return false, &protectedInspectionExpiry{cause: ctx.Err()}
		case result := <-done:
			if err := ctx.Err(); err != nil {
				return false, &protectedInspectionExpiry{cause: err} // A late clear result never admits input.
			}
			return result.active, result.err
		}
	}
}
