//go:build windows

package inputworker

import "context"

// Advisory observations own a separate, serial sender. A synchronous WTS query
// here must not occupy the native command dispatcher. The existing bounded
// queue, per-queue sample order and shared wire sequence/write gate are unchanged.
func pumpSourceBoundaries(ctx context.Context, events <-chan sourcePacket, send func([]sourcePacket) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var first sourcePacket
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-events:
			if !ok {
				return nil
			}
			first = p
		}
		var batch [sourceBoundaryBatchLimit]sourcePacket
		n := collectSourceBoundaries(first, events, nil, &batch)
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := send(batch[:n]); err != nil {
			return err
		}
	}
}
