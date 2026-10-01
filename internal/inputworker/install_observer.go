package inputworker

import (
	"context"
	"errors"
	"syscall"

	"github.com/0disoft/krasis/internal/installationdiag"
)

// Native errors retain their numeric cause internally, never their error text.
type installNativeFailure struct {
	operation     string
	marker, cause error
}

func (e *installNativeFailure) Error() string   { return e.marker.Error() }
func (e *installNativeFailure) Unwrap() []error { return []error{e.marker, e.cause} }
func installNativeError(operation string, marker, cause error) error {
	if cause == nil {
		return marker
	}
	return &installNativeFailure{operation, marker, cause}
}
func installErrorFields(err error) (string, uint32, bool) {
	operation := ""
	var native *installNativeFailure
	if errors.As(err, &native) {
		operation = native.operation
	}
	var code syscall.Errno
	if errors.As(err, &code) && code != 0 {
		return operation, uint32(code), true
	}
	return operation, 0, false
}

// This decorator observes the unchanged transaction/rollback order. It records
// the first failure independently of a later rollback failure.
type observedInstallBackend struct {
	installBackend
	summary installationdiag.Summary
	failed  bool
	phase   installPhase
}

func (b *observedInstallBackend) observe(stage string, err error) error {
	if err == nil {
		return nil
	}
	op, code, known := installErrorFields(err)
	if !b.failed {
		b.summary.Stage, b.summary.Operation, b.summary.Win32Error, b.summary.Win32Known = stage, op, code, known
		b.failed = true
		b.summary.Phase = string(b.phase)
	} else if b.summary.RecoveryStage == "" {
		b.summary.RecoveryStage, b.summary.RecoveryOperation, b.summary.RecoveryWin32Error, b.summary.RecoveryWin32Known = stage, op, code, known
		b.summary.RecoveryPhase = string(b.phase)
	}
	return err
}
func (b *observedInstallBackend) Lock(ctx context.Context) (func(), error) {
	unlock, err := b.installBackend.Lock(ctx)
	return unlock, b.observe("lock", err)
}
func (b *observedInstallBackend) Load(ctx context.Context) (*installJournal, error) {
	value, err := b.installBackend.Load(ctx)
	if value != nil {
		b.phase = value.Phase
	}
	return value, b.observe("load", err)
}
func (b *observedInstallBackend) Snapshot(ctx context.Context) (installSnapshot, error) {
	value, err := b.installBackend.Snapshot(ctx)
	return value, b.observe("snapshot", err)
}
func (b *observedInstallBackend) Save(ctx context.Context, value installJournal) error {
	b.phase = value.Phase
	return b.observe("save", b.installBackend.Save(ctx, value))
}
func (b *observedInstallBackend) Stage(ctx context.Context, value installJournal) error {
	return b.observe("stage", b.installBackend.Stage(ctx, value))
}
func (b *observedInstallBackend) Stop(ctx context.Context) error {
	return b.observe("stop", b.installBackend.Stop(ctx))
}
func (b *observedInstallBackend) Replace(ctx context.Context, value installImages) error {
	return b.observe("replace", b.installBackend.Replace(ctx, value))
}
func (b *observedInstallBackend) Start(ctx context.Context) error {
	return b.observe("start", b.installBackend.Start(ctx))
}
func (b *observedInstallBackend) Verify(ctx context.Context, value installSnapshot) error {
	return b.observe("verify", b.installBackend.Verify(ctx, value))
}
func (b *observedInstallBackend) Restore(ctx context.Context, value installSnapshot) error {
	return b.observe("restore", b.installBackend.Restore(ctx, value))
}
func (b *observedInstallBackend) Finish(ctx context.Context) error {
	return b.observe("finish", b.installBackend.Finish(ctx))
}
