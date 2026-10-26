//go:build windows

package inputworker

import (
	"context"

	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/wininput"
)

func activateSourceCaptureProfile(capture wininput.SourceActivation) (handoff.CaptureNative, error) {
	ctx, probe := handoff.WithCaptureProbe(context.Background())
	var err error
	if profiled, ok := capture.(wininput.SourceProfiledActivation); ok {
		err = profiled.ActivateProfile(ctx)
	} else {
		err = capture.Activate()
	}
	return probe.Finish(), err
}
