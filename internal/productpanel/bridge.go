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
	Schema    string       `json:"schema"`
	State     string       `json:"state"`
	PeerAlias string       `json:"peerAlias"`
	Peers     []BridgePeer `json:"peers"`
	Code      string       `json:"code"`
	Notice    string       `json:"notice"`
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
func (b *DesktopBridge) Disconnect() BridgeSnapshot { return b.execute("disconnect", "", "") }

func bridgeFailure(code string) BridgeSnapshot {
	return BridgeSnapshot{Schema: BridgeSchema, State: "UNKNOWN", Peers: []BridgePeer{}, Code: code}
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
		return bridgeFailure("cancelled")
	}
	client, err := b.open()
	if err != nil || client == nil {
		return bridgeFailure("unavailable")
	}
	_, fingerprintErr := hex.DecodeString(fingerprint)
	if action == "connect" && (alias == "" || len(alias) > 256 || len(fingerprint) != 64 || fingerprintErr != nil) {
		return bridgeFailure("invalid_peer")
	}
	guard := &bridgeClient{Client: client, fingerprint: fingerprint}
	view := Execute(ctx, func() (Client, error) { return guard, nil }, action, alias)
	if ctx.Err() != nil {
		return bridgeFailure("cancelled")
	}
	if guard.stale {
		return bridgeFailure("stale_peer")
	}
	result := BridgeSnapshot{Schema: BridgeSchema, State: view.State, PeerAlias: view.PeerAlias,
		Peers: []BridgePeer{}, Code: "ok", Notice: view.Notice}
	if action != "refresh" && (!guard.attempted || guard.rejected) {
		result.Code = "rejected"
	}
	if guard.uncertain {
		result.Code = "outcome_unknown"
	}
	if result.State == "" {
		result.State = "UNKNOWN"
		result.Code = "unavailable"
	}
	if result.State == "FAULTED" {
		result.Code = "recovery_required"
	}
	for _, name := range view.Peers {
		if fp := view.Fingerprints[name]; len(fp) == 64 {
			result.Peers = append(result.Peers, BridgePeer{Alias: name, Fingerprint: fp})
		}
	}
	return result
}

type bridgeClient struct {
	Client
	fingerprint                    string
	stale                          bool
	attempted, rejected, uncertain bool
}

// Recheck the selected identity immediately before handing the alias to the
// existing agent API. The agent remains the final trust/permission authority.
func (c *bridgeClient) Connect(ctx context.Context, alias string) (result localapi.OperationResult, err error) {
	trust, err := c.Client.Trust(ctx)
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
	result, err = c.Client.Connect(ctx, alias)
	c.uncertain = err != nil
	c.rejected = err == nil && result.Status != "COMPLETED"
	return result, err
}

func (c *bridgeClient) Disconnect(ctx context.Context) (localapi.OperationResult, error) {
	c.attempted = true
	result, err := c.Client.Disconnect(ctx)
	c.uncertain = err != nil
	c.rejected = err == nil && result.Status != "COMPLETED"
	return result, err
}
