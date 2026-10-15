//go:build windows

package inputworker

import (
	"context"
	"errors"
	"time"
)

const sourcePipeName = `\\.\pipe\Krasis.Source.v1`

// One serial source endpoint per service/console, independent of the input
// endpoint. Neither accepts the other mode. Cleanup uncertainty stops both.
func runSourceService(ctx context.Context, installation *Installation, admit func(context.Context) error, observe func(Report)) error {
	agent, service, err := installation.digests()
	if err != nil {
		return err
	}
	for ctx.Err() == nil {
		channel, err := acceptAgentNamed(ctx, agent, nil, installation, nil, sourcePipeName)
		if err == nil {
			var report Report
			if admit != nil {
				err = admit(ctx)
			}
			if err == nil {
				lease, prepare, leaseErr := receiveAgentRequest(ctx, channel)
				if leaseErr == nil && !prepare && lease.Capture && !lease.Presentation {
					report, err = runPreparedAgentLease(ctx, channel, lease, agent, service, installation, nil)
				}
			}
			closeErr := channel.Close()
			if observe != nil && report.Source != nil {
				observe(report) // Once per authenticated final report, after close.
			}
			if failure := serviceRecoveryFailure(err, closeErr); failure != nil {
				return failure
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil
}

func runServiceWithSource(ctx context.Context, installation *Installation, admit func(context.Context) error, regular func(context.Context) error, observe func(Report)) error {
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := runSourceService(run, installation, admit, observe)
		cancel()
		done <- serviceEndpointFailure(err, "capture")
	}()
	err := serviceEndpointFailure(regular(run), "input")
	cancel()
	return errors.Join(err, <-done)
}
