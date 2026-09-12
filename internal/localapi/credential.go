package localapi

import (
	"context"
	"net/http"

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
)

const CredentialSchemaVersion = "krasis.agent.credential/v1"

type CredentialRequest struct {
	SchemaVersion       string                          `json:"schemaVersion"`
	Action              string                          `json:"action"`
	Role                transport.AuthenticatedPeerRole `json:"role"`
	ExpectedFingerprint string                          `json:"expectedFingerprint,omitempty"`
}

type CredentialResult struct {
	SchemaVersion string                         `json:"schemaVersion"`
	Status        string                         `json:"status"`
	Code          string                         `json:"code"`
	Info          *productidentity.TransportInfo `json:"info,omitempty"`
}

type credentialOperations interface {
	Credential(context.Context, CredentialRequest) (CredentialResult, error)
}

func (request CredentialRequest) Valid() bool {
	if request.SchemaVersion != CredentialSchemaVersion || (request.Role != transport.AuthenticatedPeerListener && request.Role != transport.AuthenticatedPeerConnector) {
		return false
	}
	return (request.Action == "SHOW" && request.ExpectedFingerprint == "") || (request.Action == "RENEW" && validFingerprint(request.ExpectedFingerprint))
}

func (handler *handler) credential(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
		writeMethodNotAllowed(response)
		return
	}
	var value CredentialRequest
	if decodeStrict(request.Body, &value) != nil || !value.Valid() {
		writeResult(response, http.StatusBadRequest, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "invalid_request"})
		return
	}
	operations, ok := handler.operations.(credentialOperations)
	if !ok {
		writeJSON(response, http.StatusOK, CredentialResult{SchemaVersion: CredentialSchemaVersion, Status: "REJECTED", Code: "not_configured"})
		return
	}
	result, err := operations.Credential(request.Context(), value)
	if err != nil {
		writeInternalFailure(response)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (client *Client) Credential(ctx context.Context, request CredentialRequest) (CredentialResult, error) {
	request.SchemaVersion = CredentialSchemaVersion
	var result CredentialResult
	if !request.Valid() {
		return result, ErrProtocol
	}
	err := client.request(ctx, http.MethodPost, "/v1/credential", request, &result)
	if err != nil {
		return result, err
	}
	if result.SchemaVersion != CredentialSchemaVersion {
		return result, ErrProtocol
	}
	switch result.Status {
	case "COMPLETED":
		if result.Info == nil || !validFingerprint(result.Info.Fingerprint) || result.Info.Role != request.Role || result.Info.NotBefore.IsZero() || !result.Info.NotAfter.After(result.Info.NotBefore) {
			return result, ErrProtocol
		}
	case "REJECTED", "FAILED":
		if result.Info != nil {
			return result, ErrProtocol
		}
	default:
		return result, ErrProtocol
	}
	return result, nil
}
