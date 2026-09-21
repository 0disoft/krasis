//go:build windows && desktop

package main

import "github.com/0disoft/krasis/internal/productpanel"

// This service intentionally has no SEND/APPLY action or bundle path argument.
func (s *WorkspaceService) UpdateSettings(peer string) productpanel.UpdateSettingsSnapshot {
	return s.updates.Status(peer)
}
func (s *WorkspaceService) UpdatePermission(peer string, allow bool) productpanel.UpdateSettingsSnapshot {
	return s.updates.Permit(peer, allow)
}
