package inputworker

import "context"

// A synchronous authority/desktop executor can fail before calling the input
// function. Only that proven pre-dispatch failure is NOT_APPLIED. Once the
// callback begins, preserve its original outcome, including unknown or partial
// application and errors returned by the executor after callback completion.
// This wrapper never retries an operation.
func beforeInputCertainty(execute executor) executor {
	return func(ctx context.Context, operation func() error) error {
		started := false
		err := execute(ctx, func() error {
			started = true
			return operation()
		})
		if err != nil && !started {
			return notSent(err)
		}
		return err
	}
}
