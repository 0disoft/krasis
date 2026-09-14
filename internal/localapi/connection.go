package localapi

import (
	"context"
	"net/http"

	"github.com/0disoft/krasis/internal/truststore"
)

const ConnectionSchemaVersion = "krasis.agent.connection/v1"

type ConnectionRequest struct {
	SchemaVersion string `json:"schemaVersion"`
	Action        string `json:"action"`
	PeerAlias     string `json:"peerAlias"`
	Address       string `json:"address,omitempty"`
	Edge          string `json:"edge,omitempty"`
	Fingerprint   string `json:"fingerprint,omitempty"`
	RearmDistance int32  `json:"rearmDistance,omitempty"`
}

type ConnectionResult struct {
	SchemaVersion string                        `json:"schemaVersion"`
	Status        string                        `json:"status"`
	Code          string                        `json:"code"`
	Profile       *truststore.ConnectionProfile `json:"profile,omitempty"`
}

type connectionOperations interface {
	Connection(context.Context, ConnectionRequest) (ConnectionResult, error)
}

func (request ConnectionRequest) Valid() bool {
	if request.SchemaVersion != ConnectionSchemaVersion || !validAlias(request.PeerAlias) {
		return false
	}
	if request.Action != "SET_REARM" && request.RearmDistance != 0 {
		return false
	}
	switch request.Action {
	case "SET_REARM":
		return request.Address == "" && request.Edge == "" && validFingerprint(request.Fingerprint) && truststore.ValidRearm(request.RearmDistance)
	case "SHOW":
		return request.Address == "" && request.Edge == "" && request.Fingerprint == ""
	case "SET":
		return request.Fingerprint == "" && (truststore.ConnectionSettings{Address: request.Address, Edge: request.Edge}).Validate() == nil
	case "SET_EDGE":
		return request.Address == "" && validFingerprint(request.Fingerprint) && (request.Edge == "LEFT" || request.Edge == "RIGHT")
	default:
		return false
	}
}

func (handler *handler) connection(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
		writeMethodNotAllowed(response)
		return
	}
	var value ConnectionRequest
	if decodeStrict(request.Body, &value) != nil || !value.Valid() {
		writeResult(response, http.StatusBadRequest, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "invalid_request"})
		return
	}
	operations, ok := handler.operations.(connectionOperations)
	if !ok {
		writeJSON(response, http.StatusOK, ConnectionResult{SchemaVersion: ConnectionSchemaVersion, Status: "REJECTED", Code: "not_configured"})
		return
	}
	result, err := operations.Connection(request.Context(), value)
	if err != nil {
		writeInternalFailure(response)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (client *Client) Connection(ctx context.Context, request ConnectionRequest) (ConnectionResult, error) {
	request.SchemaVersion = ConnectionSchemaVersion
	var result ConnectionResult
	if !request.Valid() {
		return result, ErrProtocol
	}
	err := client.request(ctx, http.MethodPost, "/v1/connection", request, &result)
	if err == nil && result.SchemaVersion != ConnectionSchemaVersion {
		err = ErrProtocol
	}
	if err == nil {
		switch result.Status {
		case "COMPLETED":
			if result.Profile == nil || result.Profile.Validate() != nil {
				err = ErrProtocol
			}
		case "REJECTED", "FAILED":
			if result.Profile != nil {
				err = ErrProtocol
			}
		default:
			err = ErrProtocol
		}
	}
	return result, err
}
