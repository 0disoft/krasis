package productcli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

const updateStatusSchema = "krasis.cli.update-status/v1"

type updateStatusResult struct {
	SchemaVersion    string `json:"schemaVersion"`
	PeerAlias        string `json:"peerAlias"`
	LocalGeneration  int    `json:"localGeneration"`
	LocalBuild       string `json:"localBuild"`
	PeerState        string `json:"peerState"`
	PeerGeneration   int    `json:"peerGeneration,omitempty"`
	PeerBuild        string `json:"peerBuild,omitempty"`
	PeerAllowsUpdate bool   `json:"peerAllowsUpdate"`
	ReceiveAllowed   bool   `json:"receiveAllowed"`
	TransferState    string `json:"transferState"`
	TransferBytes    int64  `json:"transferBytes,omitempty"`
	TransferTotal    int64  `json:"transferTotal,omitempty"`
}

func resolvePeer(ctx context.Context, client apiClient, alias string) (string, string, error) {
	trust, err := client.Trust(ctx)
	if err != nil {
		return "", "", err
	}
	var fingerprint string
	for _, peer := range trust.Peers {
		if peer.Alias != alias || peer.Revoked {
			continue
		}
		if fingerprint != "" {
			return "", "peer_alias_ambiguous", nil
		}
		fingerprint = peer.Fingerprint
	}
	if fingerprint == "" {
		return "", "peer_not_found", nil
	}
	if !validHex(fingerprint, 64) {
		return "", "peer_not_found", nil
	}
	return fingerprint, "", nil
}

func dispatchUpdate(ctx context.Context, client apiClient, args []string, executable string) (any, int, error) {
	if len(args) == 1 && args[0] == "status" {
		return updateCall(ctx, client, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	}
	if len(args) != 2 {
		return nil, ExitUsage, usageError("update requires status [ALIAS] or send ALIAS")
	}
	peer, code, err := resolvePeer(ctx, client, args[1])
	if err != nil {
		return nil, ExitInternal, err
	}
	if code != "" {
		return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: code}, ExitUnsafeState, nil
	}
	runtime, err := client.Runtime(ctx)
	if err != nil {
		return nil, ExitInternal, err
	}
	status, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	if err != nil {
		return nil, ExitInternal, err
	}
	if status.Code != "ok" {
		return status, ExitUnsafeState, nil
	}
	inspection, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "INSPECT", Peer: peer})
	if err != nil {
		return nil, ExitInternal, err
	}
	if inspection.Code != "peer_status" || inspection.PeerInfo == nil || inspection.PeerInfo.Peer != peer {
		return inspection, ExitUnsafeState, nil
	}
	if args[0] == "status" {
		return projectUpdateStatus(args[1], peer, runtime, status.Status, *inspection.PeerInfo), ExitOK, nil
	}
	if args[0] != "send" {
		return nil, ExitUsage, usageError("update requires status [ALIAS] or send ALIAS")
	}
	if inspection.PeerInfo.State != "ONLINE" {
		return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "peer_unreachable"}, ExitUnsafeState, nil
	}
	directory := currentBundleDirectory(executable, inspection.PeerInfo.Generation)
	if directory == "" {
		return localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "bundle_missing"}, ExitUnsafeState, nil
	}
	// A lost SEND response may mean the agent queued the transfer. Never retry here.
	result, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "SEND", Peer: peer, Directory: directory})
	if err != nil {
		return nil, ExitInternal, err
	}
	if result.Code != "queued" {
		return result, ExitUnsafeState, nil
	}
	if result.Status.Peer != peer || !validHex(result.Status.ID, 32) {
		return result, ExitTransport, nil
	}
	return result, ExitOK, nil
}

func updateCall(ctx context.Context, client apiClient, request localapi.UpdateRequest) (any, int, error) {
	result, err := client.Update(ctx, request)
	if err != nil {
		return nil, ExitInternal, err
	}
	if result.Code == "ok" || result.Code == "queued" {
		return result, ExitOK, nil
	}
	return result, ExitUnsafeState, nil
}

func projectUpdateStatus(alias, peer string, runtime localapi.RuntimeInfo, status peerupdate.Status, remote peerupdate.PeerInspection) updateStatusResult {
	v := updateStatusResult{SchemaVersion: updateStatusSchema, PeerAlias: alias, LocalGeneration: runtime.Generation, LocalBuild: runtime.Build, PeerState: remote.State, TransferState: status.State, TransferBytes: status.Bytes, TransferTotal: status.Total}
	if remote.State == "ONLINE" {
		v.PeerGeneration, v.PeerBuild, v.PeerAllowsUpdate = remote.Generation, remote.Revision, remote.Allowed
	}
	for _, allowed := range status.Allowed {
		if allowed == peer {
			v.ReceiveAllowed = true
		}
	}
	return v
}

func currentBundleDirectory(executable string, peerGeneration int) string {
	directory := filepath.Dir(executable)
	if filepath.Base(executable) != "krasis-cli.exe" || !bundleManifestExists(directory) {
		return ""
	}
	if peerGeneration < 75 {
		root := filepath.Dir(directory)
		if filepath.Base(directory) == "peer-update" {
			root = filepath.Dir(root)
		}
		directory = filepath.Join(root, "g75", "peer-update")
	}
	if !bundleManifestExists(directory) {
		return ""
	}
	return directory
}

func bundleManifestExists(directory string) bool {
	info, err := os.Stat(filepath.Join(directory, "BUILD.json"))
	return err == nil && info.Mode().IsRegular()
}
