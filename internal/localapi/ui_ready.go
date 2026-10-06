package localapi

import (
	"context"
	"net/http"
	"strconv"
)

func (c *Client) UIReady(ctx context.Context, pid uint32) (OperationResult, error) {
	var r OperationResult
	err := c.requestHeaders(ctx, http.MethodPost, "/v1/ui-ready", nil, &r, map[string]string{"X-Krasis-UI-PID": strconv.FormatUint(uint64(pid), 10)})
	if err == nil && r.SchemaVersion != ResultSchemaVersion {
		err = ErrProtocol
	}
	return r, err
}
func (h *handler) uiReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !emptyBody(r.Body) {
		writeMethodNotAllowed(w)
		return
	}
	pid, err := strconv.ParseUint(r.Header.Get("X-Krasis-UI-PID"), 10, 32)
	if err != nil || pid == 0 {
		writeResult(w, http.StatusBadRequest, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "invalid_request"})
		return
	}
	op, ok := h.operations.(interface {
		UIReady(context.Context, uint32) (OperationResult, error)
	})
	if !ok {
		writeResult(w, http.StatusConflict, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "not_configured"})
		return
	}
	value, err := op.UIReady(r.Context(), uint32(pid))
	if err != nil {
		writeInternalFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
