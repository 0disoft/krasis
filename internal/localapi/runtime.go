package localapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"runtime/debug"
)

const RuntimeSchemaVersion = "krasis.agent.runtime/v1"
const AgentProtocol = 1
const AgentGeneration = 79

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

func (client *Client) StopForUpgrade(ctx context.Context, instance string) (OperationResult, error) {
	var value OperationResult
	err := client.requestHeaders(ctx, http.MethodPost, "/v1/upgrade-stop", nil, &value, map[string]string{"If-Match": instance})
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
	operations, ok := handler.operations.(interface {
		StopForUpgrade(context.Context) (OperationResult, error)
	})
	if !ok {
		writeResult(response, http.StatusConflict, OperationResult{SchemaVersion: ResultSchemaVersion, Status: "REJECTED", Code: "not_configured"})
		return
	}
	value, err := operations.StopForUpgrade(request.Context())
	if err != nil {
		writeInternalFailure(response)
		return
	}
	writeJSON(response, http.StatusOK, value)
}
