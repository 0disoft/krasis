//go:build windows && desktop

package workspacehost

import "github.com/0disoft/krasis/internal/productpanel"

// Renderer calls cannot choose bundle paths or execute received files.
func (s *WorkspaceService) UpdateSettings(peer string) productpanel.UpdateSettingsSnapshot {
	return s.updates.Status(peer)
}
func (s *WorkspaceService) UpdatePermission(peer string, allow bool) productpanel.UpdateSettingsSnapshot {
	return s.updates.Permit(peer, allow)
}

func (s *WorkspaceService) SendUpdate(peer string) productpanel.UpdateTransferSnapshot {
	return s.updates.Send(peer)
}
func (s *WorkspaceService) UpdateTransfer(peer string) productpanel.UpdateTransferSnapshot {
	return s.updates.TransferStatus(peer)
}
