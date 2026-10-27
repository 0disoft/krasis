package productpanel

import "context"

// Caller owns mu. A completed read returns that ownership without unlocking,
// so installation cannot race a second maintenance request between its proof
// and its writes. On cancellation only the read-only worker inherits the gate;
// it may publish health, but can never invoke an installer or continue apply.
func (b *inputRefresh) inspectForApply(ctx context.Context) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", true, err
	}
	reader := &inputHealthReader{done: make(chan struct{})}
	b.reader.Store(reader)
	abandoned := make(chan bool, 1)
	go func() {
		reader.state = b.inspectLocked()
		b.reader.CompareAndSwap(reader, nil)
		close(reader.done)
		if <-abandoned {
			b.mu.Unlock()
		}
	}()
	select {
	case <-ctx.Done():
		abandoned <- true
		return "", false, ctx.Err()
	case <-reader.done:
		abandoned <- false
		return reader.state, true, ctx.Err()
	}
}
