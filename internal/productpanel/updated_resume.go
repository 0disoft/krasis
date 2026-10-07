package productpanel

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

// Owned by DesktopLifecycle.gate. The existing native loop drives this request;
// no renderer timer, new worker or persisted command can replay it.
type updatedWorkspaceResume struct {
	launch         WorkspaceUpdateLaunch
	attempts       int
	next, deadline time.Time
	uncertain      bool
}

func (b *DesktopLifecycle) SetUpdatedWorkspaceResume(v WorkspaceUpdateLaunch) error {
	b.gate.Lock()
	defer b.gate.Unlock()
	if v.Alias == "" { // Public releases never invent a connection.
		return nil
	}
	revision, err := hex.DecodeString(v.Revision)
	if err != nil || len(revision) != 20 || len(v.Alias) > 256 {
		return ErrAgentIncompatible
	}
	if v.Fingerprint != "" {
		fp, err := hex.DecodeString(v.Fingerprint)
		if err != nil || len(fp) != 32 {
			return ErrAgentIncompatible
		}
	}
	b.updatedResume = &updatedWorkspaceResume{launch: v}
	return nil
}

// Caller owns the lifecycle gate. Initial preparation has its own startup
// budget; polling and later retries must never renew the reconnect window.
func (b *DesktopLifecycle) startUpdatedResumeWindow(now time.Time) {
	if r := b.updatedResume; r != nil && r.deadline.IsZero() {
		r.deadline = now.Add(90 * time.Second)
	}
}

func transientUpdateStartup(err error) bool {
	return errors.Is(err, ErrStartupBusy) || errors.Is(err, ErrAgentNotReady) ||
		errors.Is(err, localapi.ErrUnavailable) || errors.Is(err, localapi.ErrTransport) ||
		errors.Is(err, context.DeadlineExceeded)
}

// Return handled while this request owns recovery. An accepted CONNECT waits
// for the supervisor; an uncertain reply permits only read-only observation.
func (b *DesktopLifecycle) resumeUpdatedConnection(ctx context.Context, now time.Time, value BridgeSnapshot) (BridgeSnapshot, bool) {
	r := b.updatedResume
	if r == nil {
		return value, false
	}
	stop := func(code string) (BridgeSnapshot, bool) {
		b.updatedResume = nil
		value.Code = code
		return value, true
	}
	if b.ctx.Err() != nil || ctx.Err() != nil || b.updateHandedOff {
		return stop("cancelled")
	}
	if r.deadline.IsZero() {
		return value, true
	}
	// Pin legacy received-update launches on their first complete trust read.
	// New direct-launch requests already supply the pre-update fingerprint.
	verifiedPeer := false
	if value.Code == "ok" && len(value.Peers) > 0 {
		fp := ""
		for _, peer := range value.Peers {
			if peer.Alias == r.launch.Alias {
				fp = peer.Fingerprint
			}
		}
		if fp == "" || (r.launch.Fingerprint != "" && fp != r.launch.Fingerprint) {
			return stop("stale_peer")
		}
		if r.launch.Fingerprint == "" {
			r.launch.Fingerprint = fp
		}
		verifiedPeer = true
	}
	switch value.State {
	case "CONNECTED", "CONNECTING", "RECONNECTING":
		if (value.Code == "ok" || value.PeerAlias != "") && value.PeerAlias != r.launch.Alias {
			return stop("stale_peer")
		}
		if value.State == "CONNECTED" && verifiedPeer {
			b.updatedResume = nil
			return value, false
		}
	case "LOCAL", "UNKNOWN":
	case "FAULTED":
		return stop("recovery_required")
	default:
		return stop("agent_busy") // Never interrupt pairing.
	}
	if !now.Before(r.deadline) {
		return stop("recovery_exhausted")
	}
	if value.State != "LOCAL" && value.State != "UNKNOWN" {
		return value, true
	}
	if r.uncertain {
		value.Code = "outcome_unknown"
		return value, true
	}
	if now.Before(r.next) {
		return value, true
	}
	if r.attempts >= 3 {
		return stop("recovery_exhausted")
	}
	r.attempts++
	r.next = now.Add(5 * time.Second)
	attempt, cancel := context.WithDeadline(ctx, r.deadline)
	defer cancel()
	if err := b.startup(attempt); err != nil {
		return b.failedStartup(attempt, err), true
	}
	client, err := b.open()
	if err != nil || client == nil {
		return bridgeFailure("unavailable"), true
	}
	runtime, ok := client.(interface {
		Runtime(context.Context) (localapi.RuntimeInfo, error)
	})
	if !ok {
		return stop("agent_incompatible")
	}
	info, err := runtime.Runtime(attempt)
	if err != nil {
		return bridgeFailure("unavailable"), true
	}
	if info.Protocol != localapi.AgentProtocol || info.Generation != localapi.AgentGeneration || info.Build != r.launch.Revision {
		return stop("agent_incompatible")
	}
	trust, err := client.Trust(attempt)
	if err != nil {
		return bridgeFailure("unavailable"), true
	}
	fp, matches := "", 0
	for _, peer := range trust.Peers {
		if peer.Alias == r.launch.Alias && !peer.Revoked {
			fp = peer.Fingerprint
			matches++
		}
	}
	if matches != 1 || len(fp) != 64 || (r.launch.Fingerprint != "" && fp != r.launch.Fingerprint) {
		return stop("stale_peer")
	}
	if r.launch.Fingerprint == "" {
		r.launch.Fingerprint = fp
	}
	// Read fresh status/trust after startup; it may have changed during Ensure.
	bridge := NewDesktopBridge(attempt, func() (Client, error) { return client, nil })
	value = bridge.Refresh()
	if value.Code != "ok" {
		return value, true
	}
	if value.State != "LOCAL" {
		return value, true
	}
	if b.stopped.Load() || attempt.Err() != nil {
		return stop("cancelled")
	}
	value = bridge.Resume(r.launch.Alias, r.launch.Fingerprint)
	switch value.Code {
	case "outcome_unknown", "unavailable":
		r.uncertain = true
	case "cancelled":
		b.updatedResume = nil
		b.recovery.Stop()
	case "stale_peer", "invalid_peer", "recovery_required", "agent_incompatible":
		b.updatedResume = nil
	}
	return value, true
}
