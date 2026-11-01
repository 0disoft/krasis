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

const (
	diagnosticOperationTimeout = 10 * time.Second
	diagnosticStatusTimeout    = 3 * time.Second
	diagnosticFingerprintSize  = 32
)

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
	budget := diagnosticOperationTimeout
	if action == "status" {
		budget = diagnosticStatusTimeout
	}
	ctx, cancel := context.WithTimeout(b.lifetime, budget)
	defer cancel()
	if ctx.Err() != nil {
		v.Code = "cancelled"
		return v
	}
	return b.executeDiagnosticAction(ctx, action, peer, v)
}

// execute keeps the lock held across local effects and remote delivery.
func (b *DiagnosticsBridge) executeDiagnosticAction(ctx context.Context, action, peer string, v DiagnosticsSnapshot) DiagnosticsSnapshot {
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
		return b.exportDiagnostic(ctx, peer, v)
	}
	return b.remoteDiagnostic(ctx, action, peer, v)
}

func validDiagnosticExportPeer(peer string) bool {
	if peer == "" {
		return true
	}
	if len(peer) != 2*diagnosticFingerprintSize {
		return false
	}
	decoded, err := hex.DecodeString(peer)
	return err == nil && len(decoded) == diagnosticFingerprintSize
}

func (b *DiagnosticsBridge) exportDiagnostic(ctx context.Context, peer string, v DiagnosticsSnapshot) DiagnosticsSnapshot {
	if !validDiagnosticExportPeer(peer) {
		v.Code = "invalid_peer"
		return v
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
	return b.remoteDiagnostic(ctx, "export", peer, v)
}

func (b *DiagnosticsBridge) remoteDiagnostic(ctx context.Context, action, peer string, v DiagnosticsSnapshot) DiagnosticsSnapshot {
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
		return projectDiagnosticStatus(v, result)
	}
	return sendDiagnostic(ctx, client, peer, v)
}

func sendDiagnostic(ctx context.Context, client DiagnosticClient, peer string, v DiagnosticsSnapshot) DiagnosticsSnapshot {
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
	return projectDiagnosticSend(v, result, err)
}

// Projections change only their local value; paths and raw errors never escape.
func projectDiagnosticStatus(v DiagnosticsSnapshot, result localapi.UpdateResult) DiagnosticsSnapshot {
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

func projectDiagnosticSend(v DiagnosticsSnapshot, result localapi.DiagnosticTransferResult, err error) DiagnosticsSnapshot {
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
