package residentagent

import (
	"context"
	"errors"
	"os"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
)

func (operations *Operations) Credential(ctx context.Context, request localapi.CredentialRequest) (localapi.CredentialResult, error) {
	result := localapi.CredentialResult{SchemaVersion: localapi.CredentialSchemaVersion, Status: "REJECTED", Code: "invalid_request"}
	if !request.Valid() {
		return result, nil
	}
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if ctx.Err() != nil || operations.root.Err() != nil {
		result.Code = "operation_cancelled"
		return result, nil
	}
	if operations.credentials == nil {
		result.Code = "not_configured"
		return result, nil
	}
	if request.Action == "RENEW" {
		if operations.sessionDone != nil {
			result.Code = "active_session_requires_disconnect"
			return result, nil
		}
		if pairing, ok := operations.pairing.(interface{ Active() bool }); ok && pairing.Active() {
			result.Code = "pairing_already_active"
			return result, nil
		}
	}
	var info productidentity.TransportInfo
	var err error
	if request.Action == "RENEW" {
		info, err = operations.credentials.RenewTransport(request.Role, request.ExpectedFingerprint, operations.now())
	} else {
		info, err = operations.credentials.InspectRenewableTransport(request.Role, operations.now())
	}
	if err != nil {
		switch {
		case errors.Is(err, productidentity.ErrTransportFingerprintChanged):
			result.Code = "credential_changed"
		case errors.Is(err, os.ErrNotExist):
			result.Code = "credential_not_found"
		case errors.Is(err, productidentity.ErrStoreBusy):
			result.Status, result.Code = "FAILED", "credential_store_busy"
		default:
			result.Status, result.Code = "FAILED", "credential_unavailable"
		}
		return result, nil
	}
	result.Status, result.Code, result.Info = "COMPLETED", "credential_loaded", &info
	if request.Action == "RENEW" {
		result.Code = "credential_renewed_repair_required"
		operations.recordLocked(result.Code)
	}
	return result, nil
}
