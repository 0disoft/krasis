//go:build windows

package inputworker

import (
	"context"

	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/wininput"
)

func activateSourceCaptureProfile(capture wininput.SourceActivation) (handoff.CaptureNative, handoff.CaptureNativeExact, error) {
	ctx, probe := handoff.WithCaptureProbe(context.Background())
	var err error
	if profiled, ok := capture.(wininput.SourceProfiledActivation); ok {
		err = profiled.ActivateProfile(ctx)
	} else {
		err = capture.Activate()
	}
	native, exact := probe.FinishDetailed()
	return native, exact, err
}
