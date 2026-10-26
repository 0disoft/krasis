package inputworker

import "context"

// startupCancellation owns cancellation until startup hands off the channel.
// The startup goroutine must call finish before publishing success and defer it
// for failures. Finish is idempotent for that owner; it disarms the callback or
// joins a callback already closing the channel, then rejects canceled startup.
// Channel.Close still retains handles owned by an unfinished native query.
func startupCancellation(ctx context.Context, channel *Channel) (finish func() error) {
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(done)
		_ = channel.Close()
	})
	return func() error {
		if stop != nil {
			if !stop() {
				<-done
			}
			stop = nil
		}
		return ctx.Err()
	}
}
