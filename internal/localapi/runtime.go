package localapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"runtime/debug"
	"strconv"
)

const RuntimeSchemaVersion = "krasis.agent.runtime/v1"
const AgentProtocol = 1
const AgentGeneration = 102

type RuntimeInfo struct {
	SchemaVersion string `json:"schemaVersion"`
	Protocol      int    `json:"protocol"`
	Generation    int    `json:"generation"`
	Build         string `json:"build"`
	Instance      string `json:"instance"`
}

func newRuntimeInfo() (RuntimeInfo, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return RuntimeInfo{}, err
	}
	value := RuntimeInfo{SchemaVersion: RuntimeSchemaVersion, Protocol: AgentProtocol, Generation: AgentGeneration, Build: "development", Instance: hex.EncodeToString(nonce[:])}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				value.Build = setting.Value
			}
		}
	}
	return value, nil
}

func (client *Client) Runtime(ctx context.Context) (RuntimeInfo, error) {
	var value RuntimeInfo
	err := client.get(ctx, "/v1/runtime", &value)
	if err == nil && (value.SchemaVersion != RuntimeSchemaVersion || value.Protocol < 1 || value.Generation < 1 || len(value.Instance) != 32) {
		err = ErrProtocol
	}
	return value, err
}

func (client *Client) StopForUpgrade(ctx context.Context, instance string, generation int, revision string) (OperationResult, error) {
	var value OperationResult
	headers := map[string]string{"If-Match": instance}
	if generation > 0 && len(revision) == 40 {
		headers["X-Krasis-Upgrade-Generation"] = strconv.Itoa(generation)
		headers["X-Krasis-Upgrade-Build"] = revision
	}
	err := client.requestHeaders(ctx, http.MethodPost, "/v1/upgrade-stop", nil, &value, headers)
	if err == nil && value.SchemaVersion != ResultSchemaVersion {
		err = ErrProtocol
	}
	return value, err
}

func (handler *handler) upgradeStop(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || !emptyBody(request.Body) {
		writeMethodNotAllowed(response)
		return
	}
	if handler.runtime.Instance == "" || request.Header.Get("If-Match") != handler.runtime.Instance {
		writeResult(response, http.StatusConflict, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "agent_changed"})
		return
	}
	generation := 0
	revision := request.Header.Get("X-Krasis-Upgrade-Build")
	if raw := request.Header.Get("X-Krasis-Upgrade-Generation"); raw != "" {
		var err error
		generation, err = strconv.Atoi(raw)
		if err != nil || generation < 1 || generation > 1000000 {
			writeResult(response, http.StatusBadRequest, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "invalid_upgrade_target"})
			return
		}
	}
	_, revisionErr := hex.DecodeString(revision)
	if (generation == 0) != (revision == "") || (revision != "" && (len(revision) != 40 || revisionErr != nil)) {
		writeResult(response, http.StatusBadRequest, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "invalid_upgrade_target"})
		return
	}
	operations, ok := handler.operations.(interface {
		StopForUpgrade(context.Context, int, string) (OperationResult, error)
	})
	if !ok {
		writeResult(response, http.StatusConflict, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "not_configured"})
		return
	}
	value, err := operations.StopForUpgrade(request.Context(), generation, revision)
	if err != nil {
		writeInternalFailure(response)
		return
	}
	writeJSON(response, http.StatusOK, value)
}
