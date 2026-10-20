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
func runSourceService(ctx context.Context, installation *Installation, admit func(context.Context) error, observe func(Report, error)) (result error) {
	agent, service, err := installation.digests()
	if err != nil {
		return err
	}
	listener, err := newServicePipeListener(sourcePipeName, nil)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, listener.Close()) }()
	for ctx.Err() == nil {
		initial, err := listener.borrow()
		if err != nil {
			return err
		}
		channel, err := acceptAgentNamed(ctx, agent, nil, installation, initial, sourcePipeName)
		if err == nil {
			var report Report
			if admit != nil {
				err = admit(ctx)
				err = sourceStartupFailure("admission", report, err)
			}
			if err == nil {
				lease, prepare, leaseErr := receiveAgentRequest(ctx, channel)
				err = leaseErr
				err = sourceStartupFailure("request", report, err)
				if err == nil && !prepare && lease.Capture && !lease.Presentation {
					report, err = runPreparedAgentLease(ctx, channel, lease, agent, service, installation, nil)
				} else if err == nil {
					err = ErrProtocol
				}
			}
			closeErr := channel.Close()
			if observe != nil && (report.Source != nil || err != nil) {
				observe(report, err) // Preserve the joined lease result; close failure is separate.
			}
			if failure := serviceRecoveryFailure(err, closeErr); failure != nil {
				return failure
			}
			// The worker, native owners and authenticated report have joined.
			// Publish the next blocking endpoint immediately after clean recovery.
			// Rejections/errors retain backoff; cleanup uncertainty still fences.
			if err == nil {
				continue
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

func runServiceWithSource(ctx context.Context, installation *Installation, admit func(context.Context) error, regular func(context.Context) error, observe func(Report, error)) error {
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
