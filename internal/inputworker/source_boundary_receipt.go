package inputworker

import (
	"context"
	"errors"
	"time"
)

const sourceBoundaryReceiptRetry = 10 * time.Millisecond

// Only the producer, before any bytes/sequence escape, may issue this marker.
// A plain deadline or an attempted/partial write never permits a retry.
type sourceReceiptUnsent struct{ cause error }

func (e *sourceReceiptUnsent) Error() string { return "boundary receipt not submitted" }
func (e *sourceReceiptUnsent) Unwrap() error { return e.cause }

func deferSourceReceipt(op byte, err error) error {
	if op == sourceBoundaryAck && errors.Is(err, context.DeadlineExceeded) {
		return &sourceReceiptUnsent{cause: err}
	}
	return err
}

// A single bounded reader owns ordered credit receipts. Idle maintenance runs
// independently; it must not wait behind an unsent receipt's native preflight.
func runSourceBoundaryReceipts(ctx context.Context, receipts <-chan uint64, send func(context.Context, uint64) error) error {
	if ctx == nil || send == nil {
		return ErrAuthority
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case reference := <-receipts:
			if reference == 0 {
				return ErrProtocol
			}
			if err := sendSourceBoundaryReceipt(ctx, reference, send); err != nil {
				return err
			}
		}
	}
}

func sendSourceBoundaryReceipt(ctx context.Context, reference uint64, send func(context.Context, uint64) error) error {
	receipt, cancel := context.WithTimeout(ctx, IdleTimeout)
	defer cancel()
	deadline, _ := receipt.Deadline()
	// Preserve the original finite deadline across retries and finish an already
	// started command before retirement even if its capture owner is cancelled.
	request, stop := context.WithDeadline(context.WithoutCancel(receipt), deadline)
	defer stop()
	for {
		if err := receipt.Err(); err != nil {
			return err
		}
		err := send(request, reference)
		if err == nil {
			return nil
		}
		if _, unsent := err.(*sourceReceiptUnsent); !unsent {
			return err
		}
		timer := time.NewTimer(sourceBoundaryReceiptRetry)
		select {
		case <-receipt.Done():
			timer.Stop()
			return errors.Join(receipt.Err(), err)
		case <-timer.C:
		}
	}
}
