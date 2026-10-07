package productpanel

import (
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productlog"
)

const DiagnosticsSchema = "krasis.diagnostics/v1"

// DiagnosticsSnapshot exposes neither paths nor log contents. Transfer is the
// agent's most recent transfer, not the currently selected device's status.
type DiagnosticsSnapshot struct {
	Schema     string                        `json:"schema"`
	Code       string                        `json:"code"`
	Saved      bool                          `json:"saved"`
	Received   bool                          `json:"received"`
	Transfer   string                        `json:"transfer"`
	Collection *DiagnosticCollectionSnapshot `json:"collection,omitempty"`
}

type DiagnosticClient interface {
	Trust(context.Context) (localapi.Trust, error)
	Update(context.Context, localapi.UpdateRequest) (localapi.UpdateResult, error)
	SendDiagnostic(context.Context, string) (localapi.DiagnosticTransferResult, error)
}

// Native-owned hooks cannot be supplied by the renderer. Local saving works
// without a live agent; peer delivery uses the existing management channel.
type DiagnosticsBridge struct {
	lifetime       context.Context
	open           func() (DiagnosticClient, error)
	export         func() error
	files          func() (bool, bool, error)
	showFolder     func() error
	mu             sync.Mutex
	collection     *diagnosticCollection
	async          bool
	working        bool
	saveCollection func(productlog.CollectionReceipt) error
}

func NewDiagnosticsBridge(ctx context.Context, open func() (DiagnosticClient, error), export func() error, files func() (bool, bool, error), showFolder func() error) *DiagnosticsBridge {
	return &DiagnosticsBridge{lifetime: ctx, open: open, export: export, files: files, showFolder: showFolder}
}

func (b *DiagnosticsBridge) Status() DiagnosticsSnapshot            { return b.execute("status", "") }
func (b *DiagnosticsBridge) Export(peer string) DiagnosticsSnapshot { return b.execute("export", peer) }
func (b *DiagnosticsBridge) OpenFolder() DiagnosticsSnapshot        { return b.execute("open", "") }
func (b *DiagnosticsBridge) Collect() DiagnosticsSnapshot           { return b.execute("collect", "") }

func (b *DiagnosticsBridge) execute(action, peer string) DiagnosticsSnapshot {
	v := DiagnosticsSnapshot{Schema: DiagnosticsSchema, Code: "unavailable", Transfer: "UNKNOWN"}
	if b == nil || b.lifetime == nil || b.open == nil || b.export == nil || b.files == nil || b.showFolder == nil {
		return v
	}
	if !b.mu.TryLock() {
		v.Code = "busy"
		return v
	}
	defer b.mu.Unlock()
	if b.async && b.collection != nil && (action == "status" || action == "collect") {
		b.startCollectionWorker(false)
		return b.collection.snapshot()
	}
	if b.working {
		v.Code = "busy"
		return v
	}
	budget := 10 * time.Second
	if action == "status" {
		budget = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(b.lifetime, budget)
	defer cancel()
	if ctx.Err() != nil {
		v.Code = "cancelled"
		return v
	}
	if action == "collect" {
		return b.collectConnected(ctx)
	}
	if action == "status" && b.collection != nil {
		return b.pollCollection(ctx)
	}
	saved, received, err := b.files()
	v.Saved, v.Received = saved, received
	if action == "status" && err != nil {
		return v
	}
	if action == "open" {
		v.Code = "ok"
		if b.showFolder() != nil {
			v.Code = "open_failed"
		}
		return v
	}
	if action == "export" {
		if peer != "" {
			if len(peer) != 64 {
				v.Code = "invalid_peer"
				return v
			}
			decoded, err := hex.DecodeString(peer)
			if err != nil || len(decoded) != 32 {
				v.Code = "invalid_peer"
				return v
			}
		}
		if b.export() != nil {
			v.Code = "save_failed"
			return v
		}
		v.Saved = true
		if peer == "" {
			v.Code = "ok"
			return v
		}
	}
	if ctx.Err() != nil {
		v.Code = "cancelled"
		return v
	}
	client, err := b.open()
	if err != nil || client == nil {
		return v
	}
	if action == "status" {
		result, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
		if err != nil {
			return v
		}
		if result.Code != "ok" {
			if result.Code == "unsupported" {
				v.Code = "unsupported"
			}
			return v
		}
		v.Code = "ok"
		switch result.Status.Diagnostic {
		case "":
			v.Transfer = "NONE"
		case "QUEUED", "SENT", "EXPIRED", "UNSUPPORTED", "REJECTED":
			v.Transfer = result.Status.Diagnostic
		}
		return v
	}
	trust, err := client.Trust(ctx)
	if err != nil {
		return v
	}
	trusted := false
	for _, p := range trust.Peers {
		if p.Fingerprint == peer && !p.Revoked {
			trusted = true
			break
		}
	}
	if !trusted {
		v.Code = "stale_peer"
		return v
	}
	if ctx.Err() != nil {
		v.Code = "cancelled"
		return v
	}
	result, err := client.SendDiagnostic(ctx, peer)
	if err != nil {
		v.Code = "outcome_unknown"
		return v
	}
	switch result.Code {
	case "queued":
		v.Code = "queued"
		v.Transfer = "QUEUED"
	case "rejected", "unsupported", "send_failed", "save_failed":
		v.Code = result.Code
	default:
		v.Code = "outcome_unknown"
	}
	return v
}
