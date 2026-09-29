package productcli

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

const diagnosticCollectionSchema = "krasis.cli.diagnostics-collection/v1"

type diagnosticCollectionResult struct {
	SchemaVersion string `json:"schemaVersion"`
	Code          string `json:"code"`
	PeerAlias     string `json:"peerAlias"`
	ID            string `json:"id,omitempty"`
	State         string `json:"state,omitempty"`
	LocalPath     string `json:"localPath,omitempty"`
	PeerPath      string `json:"peerPath,omitempty"`
}

func collectDiagnostics(ctx context.Context, client apiClient, args []string) (any, int, error) {
	result := diagnosticCollectionResult{SchemaVersion: diagnosticCollectionSchema, Code: "rejected", PeerAlias: args[1]}
	peer, code, err := resolvePeer(ctx, client, args[1])
	if err != nil {
		return nil, ExitInternal, err
	}
	if code != "" {
		result.Code = code
		return result, ExitUnsafeState, nil
	}
	if args[0] == "collect-status" {
		return collectionStatus(ctx, client, result, peer, args[2])
	}
	started, err := client.CollectDiagnostic(ctx, localapi.DiagnosticCollectionRequest{Action: "START", Peer: peer})
	if err != nil {
		result.Code = "outcome_unknown"
		return result, ExitUnsafeState, nil
	}
	if started.Code != "queued" || !validHex(started.ID, 32) {
		result.Code = started.Code
		if started.Code == "unsupported" {
			return result, ExitPolicyDenied, nil
		}
		if started.Code == "save_failed" {
			return result, ExitInternal, nil
		}
		return result, ExitUnsafeState, nil
	}
	result.ID, result.LocalPath, result.State, result.Code = started.ID, started.LocalPath, "PENDING", "pending"
	await, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-await.Done():
			return result, ExitUnsafeState, nil
		case <-ticker.C:
			value, exit, _ := collectionStatus(await, client, result, peer, started.ID)
			result = value.(diagnosticCollectionResult)
			if result.Code != "pending" {
				return result, exit, nil
			}
		}
	}
}

func collectionStatus(ctx context.Context, client apiClient, result diagnosticCollectionResult, peer, id string) (any, int, error) {
	result.ID = id
	status, err := client.CollectDiagnostic(ctx, localapi.DiagnosticCollectionRequest{Action: "STATUS", Peer: peer, ID: id})
	if err != nil {
		result.Code = "outcome_unknown"
		return result, ExitUnsafeState, nil
	}
	if status.Code != "ok" {
		result.Code = status.Code
		if status.Code == "unsupported" {
			return result, ExitPolicyDenied, nil
		}
		return result, ExitUnsafeState, nil
	}
	result.State, result.LocalPath, result.PeerPath = status.State, status.LocalPath, status.PeerPath
	switch status.State {
	case "RECEIVED":
		if status.LocalPath == "" || status.PeerPath == "" {
			result.Code = "outcome_unknown"
			return result, ExitUnsafeState, nil
		}
		result.Code = "saved"
		return result, ExitOK, nil
	case "PENDING":
		result.Code = "pending"
	default:
		result.Code = "not_received"
	}
	return result, ExitUnsafeState, nil
}
