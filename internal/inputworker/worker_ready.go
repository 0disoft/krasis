package inputworker

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

// Readiness may spend only the remaining lease/owner budget on a fresh proof
// and its bytes together. Interrupt writes without closing the independent
// report transport; join cancellation before that transport can be reused.
func publishWorkerReady(ctx context.Context, service *Channel, stream startupPublisher, expires time.Time, profile handoff.StartupProfile) error {
	return publishAuthorizedReady(ctx, service, authorityService, stream, expires, profile)
}

// Each fixed caller selects its required peer role; neither role permits a
// presence-only check to authorize a READY receipt.
func publishAuthorizedReady(ctx context.Context, owner *Channel, role authorityKind, stream startupPublisher, expires time.Time, profile handoff.StartupProfile) (result error) {
	if ctx == nil || owner == nil || owner.authority == nil || owner.authority.kind != role ||
		(role != authorityAgent && role != authorityService) || stream == nil {
		return ErrAuthority
	}
	deadline := time.Now().Add(time.Second)
	if expires.Before(deadline) {
		deadline = expires
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	deadline, _ = ctx.Deadline()
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(done)
		_ = stream.SetWriteDeadline(time.Now())
	})
	defer func() {
		if !stop() {
			<-done
		}
		err := ctx.Err()
		// Pipe and context timers can fire in either order at the same
		// deadline. Neither ordering may publish success or lose its cause.
		if err == nil && !time.Now().Before(deadline) {
			err = context.DeadlineExceeded
		}
		if err != nil {
			result = errors.Join(ErrAuthority, result, err)
		}
	}()
	started := time.Now()
	var err error
	if role == authorityAgent {
		err = owner.authorizeStartup(ctx)
	} else {
		err = owner.authorizeInput(ctx, false)
	}
	if err != nil {
		return errors.Join(ErrAuthority, err)
	}
	if role == authorityAgent {
		profile.ServiceReadiness.Record(handoff.ServiceReadyAuthorization, time.Since(started))
	}
	if err := stream.SetWriteDeadline(deadline); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	if err := writeStartupReady(stream, profile); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	if err := stream.SetWriteDeadline(time.Time{}); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	return nil
}
