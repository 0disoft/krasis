package agent

import (
	"context"

	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

func runSourceLocalReturns(ctx context.Context, source *wininput.SourceSession, outbound *peer.OutboundController) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case generation := <-source.LocalReturns():
			cleanup, cancel := context.WithTimeout(ctx, wininput.SourceCleanupTimeout)
			err := outbound.YieldLocal(cleanup)
			cancel()
			if err != nil {
				return err // The session supervisor preserves the existing failure path.
			}
			source.CompleteLocalReturn(generation)
		}
	}
}
