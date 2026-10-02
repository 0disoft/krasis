package productcli

import (
	"context"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productlog"
)

const diagnosticsExportSchema = "krasis.cli.diagnostics-export/v1"
const diagnosticTransferSchema = "krasis.cli.diagnostic-transfer/v1"

type diagnosticsExportResult struct {
	SchemaVersion string `json:"schemaVersion"`
	Code          string `json:"code"`
	Path          string `json:"path,omitempty"`
}

// Transfer is the agent's latest diagnostic job, not a receipt for a given peer.
type diagnosticTransferResult struct {
	SchemaVersion string `json:"schemaVersion"`
	Code          string `json:"code"`
	PeerAlias     string `json:"peerAlias,omitempty"`
	Transfer      string `json:"transfer,omitempty"`
}

func exportDiagnostics(localAppData string) (diagnosticsExportResult, int) {
	result := diagnosticsExportResult{SchemaVersion: diagnosticsExportSchema, Code: "save_failed"}
	path, err := productlog.Export(localAppData)
	if err != nil {
		return result, ExitInternal
	}
	result.Code, result.Path = "saved", path
	return result, ExitOK
}

func dispatchDiagnostics(ctx context.Context, client apiClient, args []string) (any, int, error) {
	if len(args) == 0 {
		value, err := client.Diagnostics(ctx)
		return value, ExitOK, err
	}
	if args[0] == "wait" {
		return dispatchDiagnosticWait(ctx, client, args)
	}
	if len(args) >= 2 && (args[0] == "collect" || args[0] == "collect-status") {
		return collectDiagnostics(ctx, client, args)
	}
	result := diagnosticTransferResult{SchemaVersion: diagnosticTransferSchema, Code: "unavailable"}
	if len(args) == 1 && args[0] == "status" {
		status, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
		if err != nil {
			return nil, ExitInternal, err
		}
		if status.Code != "ok" {
			if status.Code == "unsupported" {
				result.Code = "unsupported"
				return result, ExitPolicyDenied, nil
			}
			return result, ExitUnsafeState, nil
		}
		result.Code = "ok"
		result.Transfer = diagnosticTransferState(status.Status.Diagnostic)
		return result, ExitOK, nil
	}
	if len(args) != 2 || args[0] != "send" {
		return nil, ExitUsage, usageError("diagnostics requires export, status, or send ALIAS")
	}
	result.PeerAlias = args[1]
	peer, code, err := resolvePeer(ctx, client, args[1])
	if err != nil {
		return nil, ExitInternal, err
	}
	if code != "" {
		result.Code = code
		return result, ExitUnsafeState, nil
	}
	status, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	if err != nil {
		return nil, ExitInternal, err
	}
	if status.Code != "ok" {
		if status.Code == "unsupported" {
			result.Code = "unsupported"
			return result, ExitPolicyDenied, nil
		}
		return result, ExitUnsafeState, nil
	}
	if status.Status.Diagnostic == "QUEUED" {
		result.Code, result.Transfer = "already_queued", "QUEUED"
		return result, ExitUnsafeState, nil
	}
	// A lost response can follow a successful queue operation. Never retry it here.
	queued, err := client.SendDiagnostic(ctx, peer)
	if err != nil {
		result.Code = "outcome_unknown"
		return result, ExitUnsafeState, nil
	}
	switch queued.Code {
	case "queued":
		result.Code, result.Transfer = "queued", "QUEUED"
		return result, ExitOK, nil
	case "unsupported":
		result.Code = queued.Code
		return result, ExitPolicyDenied, nil
	case "save_failed":
		result.Code = queued.Code
		return result, ExitInternal, nil
	case "rejected", "send_failed":
		result.Code = queued.Code
		return result, ExitUnsafeState, nil
	default:
		result.Code = "outcome_unknown"
		return result, ExitUnsafeState, nil
	}
}

func diagnosticTransferState(state string) string {
	switch state {
	case "QUEUED", "SENT", "EXPIRED", "UNSUPPORTED", "REJECTED":
		return state
	case "":
		return "NONE"
	default:
		return "UNKNOWN"
	}
}
