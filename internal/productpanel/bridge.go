package productpanel

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

const BridgeSchema = "krasis.desktop/v1"

// BridgeSnapshot deliberately excludes credentials, endpoint addresses and paths.
// Notice is legacy localized panel copy; native integration must map it to catalog
// codes before advertising complete localization of agent failures.
type BridgeSnapshot struct {
	agentPrepared    bool         // Native lifecycle proof, never renderer input/output.
	InputRefreshCode string       `json:"inputRefreshCode,omitempty"`
	ReconnectCode    string       `json:"reconnectCode,omitempty"`
	ResumeState      string       `json:"resumeState,omitempty"`
	BoundaryBlocked  string       `json:"boundaryBlocked,omitempty"`
	InputSetup       string       `json:"inputSetup,omitempty"`
	InputBackend     string       `json:"inputBackend"`
	Revision         uint64       `json:"revision,omitempty"`
	Schema           string       `json:"schema"`
	State            string       `json:"state"`
	PeerAlias        string       `json:"peerAlias"`
	StopReason       string       `json:"stopReason,omitempty"`
	Peers            []BridgePeer `json:"peers"`
	Code             string       `json:"code"`
	Notice           string       `json:"notice"`
}

type BridgePeer struct {
	Alias       string `json:"alias"`
	Fingerprint string `json:"fingerprint"`
}

// DesktopBridge is owned by the native window, not the input worker. Its host
// supplies the lifetime context and credential opener; neither is browser input.
// No goroutine, network listener or process restart is started here.
type DesktopBridge struct {
	lifetime context.Context
	open     func() (Client, error)
	mu       sync.Mutex
}

func NewDesktopBridge(lifetime context.Context, open func() (Client, error)) *DesktopBridge {
	return &DesktopBridge{lifetime: lifetime, open: open}
}

func (b *DesktopBridge) Refresh() BridgeSnapshot { return b.execute("refresh", "", "") }
func (b *DesktopBridge) Connect(alias, fingerprint string) BridgeSnapshot {
	return b.execute("connect", alias, fingerprint)
}
func (b *DesktopBridge) Resume(alias, fingerprint string) BridgeSnapshot {
	return b.execute("resume", alias, fingerprint)
}
func (b *DesktopBridge) Disconnect() BridgeSnapshot { return b.execute("disconnect", "", "") }
func (b *DesktopBridge) PauseForMaintenance() BridgeSnapshot {
	return b.execute("maintenance_stop", "", "")
}

func bridgeFailure(code string) BridgeSnapshot {
	return BridgeSnapshot{Schema: BridgeSchema, State: "UNKNOWN", InputBackend: "UNKNOWN", Peers: []BridgePeer{}, Code: code}
}

func (b *DesktopBridge) execute(action, alias, fingerprint string) BridgeSnapshot {
	if b == nil || b.lifetime == nil || b.open == nil {
		return bridgeFailure("unavailable")
	}
	// Busy is visible to callers: never queue a stale connect behind disconnect.
	if !b.mu.TryLock() {
		return bridgeFailure("busy")
	}
	defer b.mu.Unlock()
	ctx, cancel := context.WithTimeout(b.lifetime, 10*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return bridgeInterruption(ctx, false, false)
	}
	client, err := b.open()
	if err != nil || client == nil {
		return bridgeFailure("unavailable")
	}
	_, fingerprintErr := hex.DecodeString(fingerprint)
	if (action == "connect" || action == "resume") && (alias == "" || len(alias) > 256 || len(fingerprint) != 64 || fingerprintErr != nil) {
		return bridgeFailure("invalid_peer")
	}
	guard := &bridgeClient{Client: client, fingerprint: fingerprint, automaticResume: action == "resume", maintenanceStop: action == "maintenance_stop"}
	if action == "maintenance_stop" {
		action = "disconnect"
	}
	if action == "resume" {
		action = "connect"
	}
	view := Execute(ctx, func() (Client, error) { return guard, nil }, action, alias)
	if ctx.Err() != nil {
		if guard.resumeCancelled {
			return bridgeFailure("cancelled") // Authoritative manual-stop refusal.
		}
		return bridgeInterruption(ctx, guard.attempted, guard.rejected)
	}
	if guard.stale {
		return bridgeFailure("stale_peer")
	}
	result := BridgeSnapshot{Schema: BridgeSchema, State: view.State, PeerAlias: view.PeerAlias, StopReason: view.StopReason,
		ResumeState:  guard.resumeState,
		InputBackend: guard.inputBackend, BoundaryBlocked: guard.boundaryBlocked,
		Peers: []BridgePeer{}, Code: "ok", Notice: view.Notice}
	if action != "refresh" && (!guard.attempted || guard.rejected) {
		result.Code = "rejected"
	}
	if result.State == "" {
		result.State = "UNKNOWN"
		result.Code = "unavailable"
	}
	if guard.trustFailed {
		result.Code = "unavailable"
	}
	if guard.uncertain {
		result.Code = "outcome_unknown"
	}
	if guard.resumeCancelled {
		result.Code = "cancelled"
	}
	if guard.resumeUnsupported {
		result.Code = "agent_incompatible"
	}
	if result.State == "FAULTED" && result.Code != "unavailable" && result.Code != "outcome_unknown" {
		result.Code = "recovery_required"
	}
	for _, name := range view.Peers {
		if fp := view.Fingerprints[name]; len(fp) == 64 {
			result.Peers = append(result.Peers, BridgePeer{Alias: name, Fingerprint: fp})
		}
	}
	return result
}

func bridgeInterruption(ctx context.Context, attempted, rejected bool) BridgeSnapshot {
	if attempted && !rejected {
		return bridgeFailure("outcome_unknown")
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return bridgeFailure("unavailable")
	}
	return bridgeFailure("cancelled")
}

type bridgeClient struct {
	Client
	resumeState                                         string
	inputBackend                                        string
	boundaryBlocked                                     string
	fingerprint                                         string
	stale                                               bool
	trustFailed                                         bool
	automaticResume, resumeCancelled, resumeUnsupported bool
	maintenanceStop                                     bool
	attempted, rejected, uncertain                      bool
}

func (c *bridgeClient) Trust(ctx context.Context) (localapi.Trust, error) {
	v, err := c.Client.Trust(ctx)
	if err != nil {
		c.trustFailed = true
	}
	return v, err
}

func (c *bridgeClient) Status(ctx context.Context) (localapi.Status, error) {
	v, err := c.Client.Status(ctx)
	// A successful command followed by an unreadable status is not a safe
	// pre-send failure. Retain uncertainty rather than invite another command.
	if err != nil && c.attempted && !c.rejected {
		c.uncertain = true
	}
	c.inputBackend = "UNKNOWN"
	c.resumeState = "UNKNOWN"
	if err == nil && (v.ResumeState == "SAVED" || v.ResumeState == "FAILED") {
		c.resumeState = v.ResumeState
	}
	c.boundaryBlocked = ""
	if err == nil {
		switch v.BoundaryBlocked {
		case "LOCAL_MENU", "MOUSE_CAPTURE", "PRESSED_INPUT", "MOVE_SIZE":
			c.boundaryBlocked = v.BoundaryBlocked
		}
	}
	if err == nil && (v.InputBackend == "PROTECTED" || v.InputBackend == "PORTABLE") {
		c.inputBackend = v.InputBackend
	}
	return v, err
}

// Recheck the selected identity immediately before handing the alias to the
// existing agent API. The agent remains the final trust/permission authority.
func (c *bridgeClient) Connect(ctx context.Context, alias string) (result localapi.OperationResult, err error) {
	trust, err := c.Trust(ctx)
	if err != nil {
		return result, err
	}
	count := 0
	for _, peer := range trust.Peers {
		if peer.Alias == alias && !peer.Revoked {
			count++
			if peer.Fingerprint != c.fingerprint {
				c.stale = true
			}
		}
	}
	if count != 1 || c.stale {
		c.stale = true
		return result, errors.New("selected peer changed")
	}
	c.attempted = true
	ctx = localapi.WithPeerFingerprint(ctx, c.fingerprint)
	if c.automaticResume {
		resume, ok := c.Client.(interface {
			ResumeConnection(context.Context, string) (localapi.OperationResult, error)
		})
		if !ok {
			c.attempted = false
			c.resumeUnsupported = true
			return result, ErrAgentIncompatible
		}
		result, err = resume.ResumeConnection(ctx, alias)
		c.resumeCancelled = err == nil && result.Status == "REJECTED" && result.Code == "resume_cancelled"
	} else {
		result, err = c.Client.Connect(ctx, alias)
	}
	c.uncertain = err != nil
	c.rejected = err == nil && result.Status != "COMPLETED"
	if err == nil && result.Code == "selected_peer_changed" {
		c.stale = true
	}
	return result, err
}

func (c *bridgeClient) Disconnect(ctx context.Context) (localapi.OperationResult, error) {
	if c.maintenanceStop {
		pause, ok := c.Client.(interface {
			PauseForMaintenance(context.Context) (localapi.OperationResult, error)
		})
		if !ok {
			c.resumeUnsupported = true
			return localapi.OperationResult{}, ErrAgentIncompatible
		}
		c.attempted = true
		result, err := pause.PauseForMaintenance(ctx)
		c.uncertain = err != nil
		c.rejected = err == nil && result.Status != "COMPLETED"
		return result, err
	}
	c.attempted = true
	result, err := c.Client.Disconnect(ctx)
	c.uncertain = err != nil
	c.rejected = err == nil && result.Status != "COMPLETED"
	return result, err
}
