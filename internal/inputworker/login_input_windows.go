//go:build windows

package inputworker

import (
	"context"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
)

// RunLoginInput supplies serialized input/cursor I/O only to an authenticated
// service child's cooperative connection callback. No native port escapes the
// dedicated Winlogon thread. Returning joins release recovery and desktop close.
func RunLoginInput(ctx context.Context, authorize func() error, callback func(context.Context, application.InputPort) error) (Report, error) {
	if authorize == nil {
		return Report{}, ErrAuthority
	}
	native, ok := wininput.NewInjectionPort().(loginNativeInput)
	if !ok {
		return Report{}, ErrAuthority
	}
	return runLoginInput(ctx, native, func(lifetime context.Context, run func(executor) error) error {
		return inputdesktop.RunWinlogon(lifetime, authorize, func(scope *inputdesktop.Scope) error { return run(scope.Do) })
	}, callback)
}
