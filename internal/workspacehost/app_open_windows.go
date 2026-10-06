//go:build windows && desktop

package workspacehost

import "github.com/0disoft/krasis/internal/productpanel"

func (s *WorkspaceService) PeerAppOpen(peer, id string) productpanel.AppOpenSnapshot {
	return s.updates.AppOpen(peer, id)
}
