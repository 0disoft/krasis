package localapi

import (
	"context"
	"encoding/hex"
	"net/http"
	"strconv"
)

func (c *Client) ReleaseClaim(ctx context.Context, instance, id string, generation int, revision string, claim bool) (OperationResult, error) {
	action := "RELEASE"
	if claim {
		action = "CLAIM"
	}
	var value OperationResult
	err := c.requestHeaders(ctx, http.MethodPost, "/v1/release-claim", nil, &value, map[string]string{
		"If-Match": instance, "X-Krasis-Release-ID": id, "X-Krasis-Release-Action": action,
		"X-Krasis-Upgrade-Generation": strconv.Itoa(generation), "X-Krasis-Upgrade-Build": revision,
	})
	if err == nil && value.SchemaVersion != ResultSchemaVersion {
		err = ErrProtocol
	}
	return value, err
}

func (h *handler) releaseClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !emptyBody(r.Body) {
		writeMethodNotAllowed(w)
		return
	}
	if h.runtime.Instance == "" || r.Header.Get("If-Match") != h.runtime.Instance {
		writeResult(w, http.StatusConflict, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "agent_changed"})
		return
	}
	id, revision, action := r.Header.Get("X-Krasis-Release-ID"), r.Header.Get("X-Krasis-Upgrade-Build"), r.Header.Get("X-Krasis-Release-Action")
	generation, err := strconv.Atoi(r.Header.Get("X-Krasis-Upgrade-Generation"))
	_, idErr := hex.DecodeString(id)
	_, revisionErr := hex.DecodeString(revision)
	if err != nil || generation <= h.runtime.Generation || generation > 1000000 || len(id) != 32 || idErr != nil || len(revision) != 40 || revisionErr != nil || (action != "CLAIM" && action != "RELEASE") {
		writeResult(w, http.StatusBadRequest, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "invalid_upgrade_target"})
		return
	}
	ops, ok := h.operations.(interface {
		ReleaseClaim(context.Context, string, int, string, bool) (OperationResult, error)
	})
	if !ok {
		writeResult(w, http.StatusConflict, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "not_configured"})
		return
	}
	value, err := ops.ReleaseClaim(r.Context(), id, generation, revision, action == "CLAIM")
	if err != nil {
		writeInternalFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
