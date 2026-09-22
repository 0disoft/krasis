//go:build windows && desktop

package workspacehost

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productpanel"
)

func (s *WorkspaceService) live() bool {
	return s != nil && s.managed != nil && s.pairs != nil
}

func (s *WorkspaceService) PairStatus() productpanel.PairingSnapshot {
	if !s.live() {
		return productpanel.PairingSnapshot{Schema: productpanel.PairingSchema, Code: "unavailable", Candidates: []productpanel.PairingCandidate{}}
	}
	return s.afterPair(s.pairs.Status())
}

func (s *WorkspaceService) PairInvite() productpanel.PairingSnapshot {
	if !s.live() {
		return productpanel.PairingSnapshot{Schema: productpanel.PairingSchema, Code: "unavailable", Candidates: []productpanel.PairingCandidate{}}
	}
	return s.afterPair(s.pairs.Invite())
}

func (s *WorkspaceService) PairSearch() productpanel.PairingSnapshot {
	if !s.live() {
		return productpanel.PairingSnapshot{Schema: productpanel.PairingSchema, Code: "unavailable", Candidates: []productpanel.PairingCandidate{}}
	}
	return s.pairs.Search()
}

func (s *WorkspaceService) PairJoin(id, pin string) productpanel.PairingSnapshot {
	if !s.live() {
		return productpanel.PairingSnapshot{Schema: productpanel.PairingSchema, Code: "unavailable", Candidates: []productpanel.PairingCandidate{}}
	}
	return s.afterPair(s.pairs.Join(id, pin))
}

func (s *WorkspaceService) PairCancel() productpanel.PairingSnapshot {
	if !s.live() {
		return productpanel.PairingSnapshot{Schema: productpanel.PairingSchema, Code: "unavailable", Candidates: []productpanel.PairingCandidate{}}
	}
	return s.pairs.Cancel()
}

func (s *WorkspaceService) afterPair(snap productpanel.PairingSnapshot) productpanel.PairingSnapshot {
	if snap.Code != "pairing_completed" || snap.Alias == "" {
		return snap
	}
	// The lifecycle snapshot may predate the trust commit by a polling interval.
	view := s.raw.Refresh()
	for _, peer := range view.Peers {
		if peer.Alias == snap.Alias {
			connected := s.Connect(peer.Alias, peer.Fingerprint)
			if connected.Code != "ok" {
				snap.Code = "paired_connect_failed"
			}
			return snap
		}
	}
	snap.Code = "paired_connect_failed"
	return snap
}

func (s *WorkspaceService) LayoutStatus(fingerprint string) productpanel.LayoutSnapshot {
	return s.layout(func(ctx context.Context) productpanel.LayoutSnapshot {
		return productpanel.LayoutStatus(ctx, s.open, fingerprint)
	})
}

func (s *WorkspaceService) LayoutSet(fingerprint, edge string) productpanel.LayoutSnapshot {
	return s.layout(func(ctx context.Context) productpanel.LayoutSnapshot {
		return productpanel.LayoutSet(ctx, s.open, fingerprint, edge)
	})
}

func (s *WorkspaceService) LayoutRearm(fingerprint string, distance int32) productpanel.LayoutSnapshot {
	return s.layout(func(ctx context.Context) productpanel.LayoutSnapshot {
		return productpanel.LayoutRearm(ctx, s.open, fingerprint, distance)
	})
}

func (s *WorkspaceService) layout(run func(context.Context) productpanel.LayoutSnapshot) productpanel.LayoutSnapshot {
	if !s.live() {
		return productpanel.LayoutSnapshot{Edge: "RIGHT", Rearm: 32, Code: "unavailable"}
	}
	ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
	defer cancel()
	return run(ctx)
}

func (s *WorkspaceService) open() (productpanel.Client, error) {
	return localapi.OpenClient(s.control)
}
