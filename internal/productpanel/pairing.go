package productpanel

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/pinauth"
	"github.com/0disoft/krasis/internal/pindiscovery"
)

const PairingSchema = "krasis.pairing/v1"
const pairingAlias = "PC"

// PairingSnapshot never includes addresses, tokens or discovery payloads.
type PairingSnapshot struct {
	Schema     string             `json:"schema"`
	Code       string             `json:"code"`
	PIN        string             `json:"pin"`
	Job        string             `json:"job"`
	Alias      string             `json:"alias"`
	Candidates []PairingCandidate `json:"candidates"`
}

type PairingCandidate struct {
	ID string `json:"id"`
}

type PairingClient interface {
	Status(context.Context) (localapi.Status, error)
	Runtime(context.Context) (localapi.RuntimeInfo, error)
	Pair(context.Context, localapi.OperationRequest) (localapi.OperationResult, error)
}

type PairingBridge struct {
	lifetime context.Context
	open     func() (PairingClient, error)
	discover func(context.Context) ([]pindiscovery.Candidate, error)
	mu       sync.Mutex
	job      string
	found    []pindiscovery.Candidate
}

func NewPairingBridge(lifetime context.Context, open func() (PairingClient, error), discover func(context.Context) ([]pindiscovery.Candidate, error)) *PairingBridge {
	if discover == nil {
		discover = pindiscovery.Discover
	}
	return &PairingBridge{lifetime: lifetime, open: open, discover: discover}
}

func pairingFailure(code string) PairingSnapshot {
	return PairingSnapshot{Schema: PairingSchema, Code: code, Candidates: []PairingCandidate{}}
}

func (b *PairingBridge) public() []PairingCandidate {
	out := make([]PairingCandidate, 0, len(b.found))
	for _, item := range b.found {
		if len(item.ID) == 32 {
			out = append(out, PairingCandidate{ID: item.ID})
		}
	}
	return out
}

func (b *PairingBridge) snapshot(op localapi.OperationResult) PairingSnapshot {
	result := PairingSnapshot{Schema: PairingSchema, Code: "ok", Job: b.job, Candidates: b.public()}
	if len(op.PairingID) == 32 {
		b.job = op.PairingID
		result.Job = op.PairingID
	}
	switch op.Code {
	case "pin_waiting":
		result.Code = "pin_waiting"
		if pinauth.Valid(op.Confirmation) {
			result.PIN = op.Confirmation
		}
	case "pairing_registering":
		result.Code = "pairing_registering"
	case "pairing_completed":
		result.Code = "pairing_completed"
		if len(b.job) == 32 {
			result.Alias = pairingAlias + "-" + b.job[:8]
		}
		b.job = ""
		result.Job = ""
	case "pairing_failed", "pairing_expired", "pairing_cancelled", "pairing_finished", "pairing_not_found", "pairing_commit_uncertain":
		b.job = ""
		result.Job = ""
		result.Code = "pairing_ended"
	case "pairing_retry_later", "pairing_already_active":
		result.Code = op.Code
	default:
		if op.Code != "" && op.Status != "COMPLETED" {
			result.Code = "rejected"
		}
	}
	return result
}

func (b *PairingBridge) client(timeout time.Duration) (context.Context, context.CancelFunc, PairingClient, PairingSnapshot) {
	if b == nil || b.lifetime == nil || b.open == nil {
		return nil, func() {}, nil, pairingFailure("unavailable")
	}
	if !b.mu.TryLock() {
		return nil, func() {}, nil, pairingFailure("busy")
	}
	ctx, cancel := context.WithTimeout(b.lifetime, timeout)
	if ctx.Err() != nil {
		cancel()
		b.mu.Unlock()
		return nil, func() {}, nil, pairingFailure("cancelled")
	}
	client, err := b.open()
	if err != nil || client == nil {
		cancel()
		b.mu.Unlock()
		return nil, func() {}, nil, pairingFailure("unavailable")
	}
	return ctx, func() { cancel(); b.mu.Unlock() }, client, PairingSnapshot{}
}

func (b *PairingBridge) ready(ctx context.Context, client PairingClient) PairingSnapshot {
	status, err := client.Status(ctx)
	if err != nil || status.State == "" {
		return pairingFailure("unavailable")
	}
	if status.State != "LOCAL" {
		return pairingFailure("not_local")
	}
	info, err := client.Runtime(ctx)
	if err != nil || info.Generation < 3 {
		return pairingFailure("agent_incompatible")
	}
	return PairingSnapshot{}
}

func (b *PairingBridge) Invite() PairingSnapshot {
	ctx, done, client, fail := b.client(20 * time.Second)
	if fail.Code != "" {
		return fail
	}
	defer done()
	if b.job != "" {
		return b.activeFailure("pairing_already_active")
	}
	if fail = b.ready(ctx, client); fail.Code != "" {
		return fail
	}
	allow := true
	op, err := client.Pair(ctx, localapi.OperationRequest{PairingAction: "PIN_LISTEN", PeerAlias: pairingAlias, AllowInboundControl: &allow})
	if err != nil {
		return pairingFailure("unavailable")
	}
	return b.snapshot(op)
}

func (b *PairingBridge) Search() PairingSnapshot {
	ctx, done, client, fail := b.client(5 * time.Second)
	if fail.Code != "" {
		return fail
	}
	defer done()
	if b.job != "" {
		return b.activeFailure("pairing_already_active")
	}
	status, err := client.Status(ctx)
	if err != nil || status.State == "" {
		return pairingFailure("unavailable")
	}
	if status.State != "LOCAL" {
		return pairingFailure("not_local")
	}
	b.found = nil
	found, err := b.discover(ctx)
	if err != nil {
		return pairingFailure("search_failed")
	}
	b.found = found
	result := PairingSnapshot{Schema: PairingSchema, Code: "ok", Job: b.job, Candidates: b.public()}
	if len(result.Candidates) == 0 {
		result.Code = "no_candidates"
	}
	return result
}

func (b *PairingBridge) Join(id, pin string) PairingSnapshot {
	if !pinauth.Valid(pin) {
		return pairingFailure("invalid_pin")
	}
	ctx, done, client, fail := b.client(20 * time.Second)
	if fail.Code != "" {
		return fail
	}
	defer done()
	if b.job != "" {
		return b.activeFailure("pairing_already_active")
	}
	var choice pindiscovery.Candidate
	for _, item := range b.found {
		if item.ID == id && len(item.ID) == 32 && item.Address != "" {
			choice = item
			break
		}
	}
	if choice.ID == "" {
		return pairingFailure("stale_candidate")
	}
	if fail = b.ready(ctx, client); fail.Code != "" {
		return fail
	}
	allow := true
	op, err := client.Pair(ctx, localapi.OperationRequest{PairingAction: "PIN_DIAL", PeerAlias: pairingAlias, AllowInboundControl: &allow, Address: choice.Address, PairingID: choice.ID, ConfirmationCode: pin})
	if err != nil {
		return pairingFailure("unavailable")
	}
	return b.snapshot(op)
}

func (b *PairingBridge) Cancel() PairingSnapshot {
	return b.pair("CANCEL", 8*time.Second)
}

func (b *PairingBridge) Status() PairingSnapshot {
	return b.pair("STATUS", 8*time.Second)
}

// Preserve the native-owned job after an ambiguous response so the user can
// query or cancel it instead of accidentally starting another invitation.
func (b *PairingBridge) activeFailure(code string) PairingSnapshot {
	return PairingSnapshot{Schema: PairingSchema, Code: code, Job: b.job, Candidates: b.public()}
}

func (b *PairingBridge) pair(action string, timeout time.Duration) PairingSnapshot {
	ctx, done, client, fail := b.client(timeout)
	if fail.Code != "" {
		return fail
	}
	defer done()
	if b.job == "" {
		return PairingSnapshot{Schema: PairingSchema, Code: "ok", Candidates: b.public()}
	}
	op, err := client.Pair(ctx, localapi.OperationRequest{PairingAction: action, PairingID: b.job})
	if err != nil {
		return b.activeFailure("unavailable")
	}
	return b.snapshot(op)
}

func (s PairingSnapshot) MarshalJSON() ([]byte, error) {
	type out PairingSnapshot
	value := out(s)
	if value.Candidates == nil {
		value.Candidates = []PairingCandidate{}
	}
	if !pinauth.Valid(value.PIN) {
		value.PIN = ""
	}
	return json.Marshal(value)
}
