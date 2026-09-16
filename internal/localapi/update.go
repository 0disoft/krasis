package localapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/0disoft/krasis/internal/peerupdate"
)

const UpdateSchema = "krasis.peer-update/v1"

type UpdateRequest struct {
	Schema    string `json:"schema"`
	Action    string `json:"action"`
	Peer      string `json:"peer,omitempty"`
	Directory string `json:"directory,omitempty"`
	ID        string `json:"id,omitempty"`
}
type UpdateResult struct {
	Schema    string            `json:"schema"`
	Code      string            `json:"code"`
	Status    peerupdate.Status `json:"status"`
	Directory string            `json:"directory,omitempty"`
	Alias     string            `json:"alias,omitempty"`
}

func (r UpdateRequest) Valid() bool {
	if r.Schema != UpdateSchema {
		return false
	}
	switch r.Action {
	case "STATUS":
		return r.Peer == "" && r.Directory == "" && r.ID == ""
	case "ALLOW", "DENY":
		return validFingerprint(r.Peer) && r.Directory == "" && r.ID == ""
	case "SEND":
		return validFingerprint(r.Peer) && r.Directory != "" && len(r.Directory) < 32768 && !strings.ContainsRune(r.Directory, 0) && r.ID == ""
	case "APPLY", "APPLY_FAILED":
		return validFingerprint(r.Peer) && validPairingID(r.ID) && r.Directory == ""
	default:
		return false
	}
}
func (client *Client) Update(ctx context.Context, request UpdateRequest) (UpdateResult, error) {
	var result UpdateResult
	err := client.request(ctx, http.MethodPost, "/v1/update", request, &result)
	if err == nil && result.Schema != UpdateSchema {
		err = ErrProtocol
	}
	return result, err
}
func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeMethodNotAllowed(w)
		return
	}
	var request UpdateRequest
	if decodeStrict(r.Body, &request) != nil || !request.Valid() {
		writeResult(w, http.StatusBadRequest, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "invalid_request"})
		return
	}
	op, ok := h.operations.(interface {
		Update(context.Context, UpdateRequest) (UpdateResult, error)
	})
	if !ok {
		writeJSON(w, http.StatusOK, UpdateResult{Schema: UpdateSchema, Code: "unsupported"})
		return
	}
	result, err := op.Update(r.Context(), request)
	if err != nil {
		writeInternalFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
