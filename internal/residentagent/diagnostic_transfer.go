package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
	"io"
	"path/filepath"
)

func (o *Operations) CollectDiagnostic(ctx context.Context, request localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error) {
	result := localapi.DiagnosticCollectionResult{Code: "rejected"}
	if o.updates == nil || !o.updateAuthorized(ctx, "", request.Peer) {
		return result, nil
	}
	if request.Action == "STATUS" {
		state, localPath, peerPath := o.updates.DiagnosticRequestStatus(request.Peer, request.ID)
		result.Code, result.ID, result.State, result.LocalPath, result.PeerPath = "ok", request.ID, state, localPath, peerPath
		return result, nil
	}
	var path string
	id, err := o.updates.PrepareDiagnosticRequest(request.Peer, func(id string) (string, error) {
		var err error
		path, err = productlog.ExportForRequest(o.localAppData, id)
		return path, err
	})
	if errors.Is(err, peerupdate.ErrBusy) {
		result.Code = "busy"
		return result, nil
	}
	if err != nil {
		result.Code = "save_failed"
		return result, nil
	}
	result.Code, result.State, result.ID, result.LocalPath = "queued", "PENDING", id, path
	return result, nil
}

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

func (o *Operations) exportDiagnosticForRequest(peer, requestID string) error {
	if o.updates == nil || !o.updateAuthorized(o.Context(), "", peer) {
		return peerupdate.ErrDenied
	}
	if o.updates.DiagnosticRequestHandled(peer, requestID) {
		return nil
	}
	path, err := productlog.ExportForRequest(o.localAppData, requestID)
	if err != nil {
		return err
	}
	dir, err := productidentity.PrepareDiagnostics(o.localAppData)
	if err != nil {
		return err
	}
	defer dir.Close()
	f, err := dir.Open(filepath.Base(path))
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, productlog.MaxSupportBytes+1))
	if err != nil {
		return err
	}
	clean, err := productlog.SanitizeSupport(data)
	if err != nil || !o.updateAuthorized(o.Context(), "", peer) {
		return peerupdate.ErrDenied
	}
	return o.updates.QueueDiagnosticForRequest(peer, requestID, clean)
}
