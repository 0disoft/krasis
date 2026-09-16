package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
	"io"
)

func (o *Operations) SendDiagnostic(ctx context.Context, peer string) (localapi.DiagnosticTransferResult, error) {
	result := localapi.DiagnosticTransferResult{Code: "rejected"}
	if o.updates == nil || !o.updateAuthorized(ctx, "", peer) {
		return result, nil
	}
	path, err := productlog.Export(o.localAppData)
	if err != nil {
		return localapi.DiagnosticTransferResult{Code: "save_failed"}, nil
	}
	result.Path = path
	result.Code = "send_failed"
	dir, err := productidentity.PrepareDiagnostics(o.localAppData)
	if err != nil {
		return result, nil
	}
	defer dir.Close()
	f, err := dir.Open(productlog.ExportName)
	if err != nil {
		return result, nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, productlog.MaxSupportBytes+1))
	if err != nil {
		return result, nil
	}
	clean, err := productlog.SanitizeSupport(data)
	if err != nil {
		return result, nil
	}
	if !o.updateAuthorized(ctx, "", peer) {
		return result, nil
	}
	if err = o.updates.QueueDiagnostic(peer, clean); err != nil {
		return result, nil
	}
	result.Code = "queued"
	return result, nil
}
