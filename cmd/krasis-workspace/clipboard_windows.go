//go:build windows && desktop

package main

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"time"
)

// ClipboardSettings intentionally has no clipboard value or content field.
type ClipboardSettings struct {
	Peer string `json:"peer"`
	Code string `json:"code"`
}

func (s *WorkspaceService) ClipboardStatus() ClipboardSettings {
	if s.managed == nil {
		return ClipboardSettings{Code: "unavailable"}
	}
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()
	client, err := localapi.OpenClient(s.control)
	if err != nil {
		return ClipboardSettings{Code: "unavailable"}
	}
	value, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	if err != nil {
		return ClipboardSettings{Code: "unavailable"}
	}
	return ClipboardSettings{Peer: value.Status.ClipboardPeer, Code: "ok"}
}

func (s *WorkspaceService) ClipboardSet(peer string, enable bool) ClipboardSettings {
	if s.managed == nil {
		return ClipboardSettings{Code: "unavailable"}
	}
	s.clipGate.Lock()
	defer s.clipGate.Unlock()
	action := "DISABLE"
	if enable {
		action = "ENABLE"
	}
	req := localapi.ClipboardRequest{Action: action, Peer: peer}
	if !req.Valid() {
		return ClipboardSettings{Code: "invalid_peer"}
	}
	// Reset even on an ambiguous transport result. A rapid off/on transition
	// must not publish text copied while sharing was disabled.
	s.clipEpoch.Add(1)
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	client, err := localapi.OpenClient(s.control)
	if err != nil {
		return ClipboardSettings{Code: "unavailable"}
	}
	// The agent is the final permission/trust authority. Only its acknowledged
	// result may change the displayed switch; no optimistic consent state.
	value, err := client.Clipboard(ctx, req)
	if err != nil || value.Code != "ok" {
		return ClipboardSettings{Code: "rejected"}
	}
	return s.ClipboardStatus()
}
