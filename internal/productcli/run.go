package productcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/0disoft/krasis/internal/localapi"
)

const (
	ExitOK               = 0
	ExitInternal         = 1
	ExitUsage            = 2
	ExitPolicyDenied     = 3
	ExitAgentUnavailable = 4
	ExitAuthentication   = 5
	ExitTransport        = 6
	ExitUnsafeState      = 7
)

type apiClient interface {
	Runtime(context.Context) (localapi.RuntimeInfo, error)
	Update(context.Context, localapi.UpdateRequest) (localapi.UpdateResult, error)
	SendDiagnostic(context.Context, string) (localapi.DiagnosticTransferResult, error)
	Clipboard(context.Context, localapi.ClipboardRequest) (localapi.ClipboardResult, error)
	Status(context.Context) (localapi.Status, error)
	Diagnostics(context.Context) (localapi.Diagnostics, error)
	Trust(context.Context) (localapi.Trust, error)
	Topology(context.Context) (localapi.Topology, error)
	Connect(context.Context, string) (localapi.OperationResult, error)
	Disconnect(context.Context) (localapi.OperationResult, error)
	Pair(context.Context, localapi.OperationRequest) (localapi.OperationResult, error)
	RevokeTrust(context.Context, string) (localapi.OperationResult, error)
	SafeStop(context.Context) (localapi.OperationResult, error)
}

type dependencies struct {
	localAppData string
	openClient   func(string) (apiClient, error)
	executable   string
}

type options struct {
	json       bool
	controlDir string
	args       []string
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer, localAppData string) int {
	return run(ctx, args, stdout, stderr, dependencies{
		localAppData: localAppData,
		openClient: func(directory string) (apiClient, error) {
			return localapi.OpenClient(directory)
		},
	})
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps dependencies) int {
	parsed, err := parseGlobalOptions(args)
	if err != nil || len(parsed.args) == 0 {
		writeUsageError(stderr, err)
		return ExitUsage
	}
	if err := validateCommandArgs(parsed.args); err != nil {
		writeUsageError(stderr, err)
		return ExitUsage
	}
	if parsed.args[0] == "help" {
		if parsed.controlDir != "" {
			writeUsageError(stderr, usageError("help does not use --control-dir"))
			return ExitUsage
		}
		return writeOutput(stdout, stderr, commandHelp(), ExitOK, parsed.json)
	}
	if parsed.args[0] == "version" {
		var client apiClient
		if control, err := ResolveControlDirectory(deps.localAppData, parsed.controlDir); err == nil {
			client, _ = deps.openClient(control)
		}
		return writeOutput(stdout, stderr, version(ctx, client), ExitOK, parsed.json)
	}
	if parsed.args[0] == "update" && parsed.args[1] == "verify" {
		executable := deps.executable
		if executable == "" {
			executable, _ = os.Executable()
		}
		value, code := verifyCurrentBundle(executable)
		return writeOutput(stdout, stderr, value, code, parsed.json)
	}
	if parsed.args[0] == "diagnostics" && len(parsed.args) == 2 && parsed.args[1] == "export" {
		if parsed.controlDir != "" {
			writeUsageError(stderr, usageError("diagnostics export does not use --control-dir"))
			return ExitUsage
		}
		value, code := exportDiagnostics(deps.localAppData)
		return writeOutput(stdout, stderr, value, code, parsed.json)
	}
	control, err := ResolveControlDirectory(deps.localAppData, parsed.controlDir)
	if err != nil {
		fmt.Fprintln(stderr, "config_error: invalid CLI configuration")
		return ExitUsage
	}
	client, err := deps.openClient(control)
	if err != nil {
		return writeClientError(stderr, err)
	}
	executable := deps.executable
	if executable == "" {
		executable, _ = os.Executable()
	}
	value, code, err := dispatch(ctx, client, parsed.args, executable)
	if err != nil {
		return writeClientError(stderr, err)
	}
	return writeOutput(stdout, stderr, value, code, parsed.json)
}

func writeOutput(stdout, stderr io.Writer, value any, code int, jsonOutput bool) int {
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(value); err != nil {
			fmt.Fprintln(stderr, "internal_error: write output")
			return ExitInternal
		}
	} else {
		writeHuman(stdout, value)
	}
	return code
}

func dispatch(ctx context.Context, client apiClient, args []string, executable string) (any, int, error) {
	switch args[0] {
	case "credential":
		return dispatchCredential(ctx, client, args[1:])
	case "connection":
		return dispatchConnection(ctx, client, args[1:])
	case "doctor":
		return doctor(ctx, client)
	case "update":
		return dispatchUpdate(ctx, client, args[1:], executable)
	case "clipboard":
		return dispatchClipboard(ctx, client, args[1:])
	case "status":
		if len(args) != 1 {
			return nil, ExitUsage, usageError("status accepts no arguments")
		}
		value, err := client.Status(ctx)
		return value, ExitOK, err
	case "diagnostics":
		return dispatchDiagnostics(ctx, client, args[1:])
	case "topology":
		if len(args) != 1 {
			return nil, ExitUsage, usageError("topology accepts no arguments")
		}
		value, err := client.Topology(ctx)
		return value, ExitOK, err
	case "connect":
		if len(args) != 2 {
			return nil, ExitUsage, usageError("connect requires a peer alias")
		}
		return operation(client.Connect(ctx, args[1]))
	case "disconnect":
		if len(args) != 1 {
			return nil, ExitUsage, usageError("disconnect accepts no arguments")
		}
		return operation(client.Disconnect(ctx))
	case "safe-stop":
		if len(args) != 1 {
			return nil, ExitUsage, usageError("safe-stop accepts no arguments")
		}
		return operation(client.SafeStop(ctx))
	case "trust":
		return dispatchTrust(ctx, client, args[1:])
	case "pair":
		return dispatchPair(ctx, client, args[1:])
	default:
		return nil, ExitUsage, usageError("unknown command")
	}
}

func dispatchTrust(ctx context.Context, client apiClient, args []string) (any, int, error) {
	if len(args) == 1 && args[0] == "list" {
		value, err := client.Trust(ctx)
		return value, ExitOK, err
	}
	if len(args) == 2 && args[0] == "revoke" {
		return operation(client.RevokeTrust(ctx, args[1]))
	}
	return nil, ExitUsage, usageError("trust requires list or revoke FINGERPRINT")
}

func dispatchPair(ctx context.Context, client apiClient, args []string) (any, int, error) {
	switch {
	case len(args) == 6 && (args[0] == "listen" || args[0] == "dial") && validInboundOption(args[3]) && args[4] == "--replace":
		allow := args[3] == "--allow-inbound-control"
		return operation(client.Pair(ctx, localapi.OperationRequest{PairingAction: strings.ToUpper(args[0]), PeerAlias: args[1], Address: args[2], AllowInboundControl: &allow, Fingerprint: args[5]}))
	case len(args) == 4 && (args[0] == "listen" || args[0] == "dial") && validInboundOption(args[3]):
		allow := args[3] == "--allow-inbound-control"
		return operation(client.Pair(ctx, localapi.OperationRequest{PairingAction: strings.ToUpper(args[0]), PeerAlias: args[1], Address: args[2], AllowInboundControl: &allow}))
	case len(args) == 2 && args[0] == "status":
		return operation(client.Pair(ctx, localapi.OperationRequest{PairingAction: "STATUS", PairingID: args[1]}))
	case len(args) == 3 && args[0] == "begin" && args[2] == "--allow-inbound-control":
		allow := true
		return operation(client.Pair(ctx, localapi.OperationRequest{PairingAction: "BEGIN", PeerAlias: args[1], AllowInboundControl: &allow}))
	case len(args) == 3 && args[0] == "confirm":
		return operation(client.Pair(ctx, localapi.OperationRequest{PairingAction: "CONFIRM", PairingID: args[1], ConfirmationCode: args[2]}))
	case len(args) == 2 && args[0] == "cancel":
		return operation(client.Pair(ctx, localapi.OperationRequest{PairingAction: "CANCEL", PairingID: args[1]}))
	default:
		return nil, ExitUsage, usageError("pair requires listen, dial, status, confirm, or cancel arguments")
	}
}

func operation(value localapi.OperationResult, err error) (any, int, error) {
	if err != nil {
		return nil, ExitInternal, err
	}
	switch value.Status {
	case "COMPLETED":
		return value, ExitOK, nil
	case "REJECTED":
		if value.Code == "not_configured" {
			return value, ExitPolicyDenied, nil
		}
		return value, ExitUnsafeState, nil
	case "FAILED":
		return value, ExitUnsafeState, nil
	default:
		return nil, ExitTransport, localapi.ErrProtocol
	}
}

func parseGlobalOptions(args []string) (options, error) {
	var result options
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			if result.json {
				return options{}, usageError("--json may be specified once")
			}
			result.json = true
		case "--help", "-h":
			result.args = append(result.args, "help")
		case "--control-dir":
			if result.controlDir != "" || index+1 >= len(args) {
				return options{}, usageError("--control-dir requires one value")
			}
			index++
			result.controlDir = args[index]
		case "--allow-inbound-control", "--deny-inbound-control", "--replace":
			result.args = append(result.args, args[index])
		default:
			if strings.HasPrefix(args[index], "--") {
				return options{}, usageError("unknown option")
			}
			result.args = append(result.args, args[index])
		}
	}
	return result, nil
}

func validateCommandArgs(args []string) error {
	switch args[0] {
	case "credential":
		_, err := parseCredential(args[1:])
		return err
	case "connection":
		_, err := parseConnection(args[1:])
		return err
	case "status", "topology", "disconnect", "safe-stop", "version", "doctor", "help":
		if len(args) != 1 {
			return usageError(args[0] + " accepts no arguments")
		}
	case "diagnostics":
		if len(args) == 1 || len(args) == 2 && (args[1] == "export" || args[1] == "status") || len(args) == 3 && args[1] == "send" && validAlias(args[2]) {
			return nil
		}
		return usageError("diagnostics requires export, status, or send ALIAS")
	case "connect":
		if len(args) != 2 || !validAlias(args[1]) {
			return usageError("connect requires a valid peer alias")
		}
	case "update":
		if len(args) == 2 && (args[1] == "status" || args[1] == "verify") {
			return nil
		}
		if len(args) != 3 || (args[1] != "status" && args[1] != "send" && args[1] != "allow" && args[1] != "deny") || !validAlias(args[2]) {
			return usageError("update requires verify, status [ALIAS], or send|allow|deny ALIAS")
		}
	case "clipboard":
		if len(args) != 3 || (args[1] != "status" && args[1] != "on" && args[1] != "off") || !validAlias(args[2]) {
			return usageError("clipboard requires status|on|off ALIAS")
		}
	case "trust":
		if len(args) == 2 && args[1] == "list" {
			return nil
		}
		if len(args) != 3 || args[1] != "revoke" || !validHex(args[2], 64) {
			return usageError("trust requires list or revoke FINGERPRINT")
		}
	case "pair":
		switch {
		case len(args) == 7 && (args[1] == "listen" || args[1] == "dial") && validAlias(args[2]) && validInboundOption(args[4]) && args[5] == "--replace" && validHex(args[6], 64):
			if _, err := parseConnection([]string{"set", args[2], args[3], "LEFT"}); err != nil {
				return err
			}
		case len(args) == 5 && (args[1] == "listen" || args[1] == "dial") && validAlias(args[2]) && validInboundOption(args[4]):
			if _, err := parseConnection([]string{"set", args[2], args[3], "LEFT"}); err != nil {
				return err
			}
		case len(args) == 3 && args[1] == "status" && validHex(args[2], 32):
		case len(args) == 4 && args[1] == "begin" && validAlias(args[2]) && args[3] == "--allow-inbound-control":
		case len(args) == 4 && args[1] == "confirm" && validHex(args[2], 32) && validDigits(args[3], 6):
		case len(args) == 3 && args[1] == "cancel" && validHex(args[2], 32):
		default:
			return usageError("pair requires valid listen, dial, status, confirm, or cancel arguments")
		}
	default:
		return usageError("unknown command")
	}
	return nil
}

func validAlias(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 64 {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validInboundOption(value string) bool {
	return value == "--allow-inbound-control" || value == "--deny-inbound-control"
}

func validHex(value string, length int) bool {
	if len(value) != length || strings.ToLower(value) != value {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validDigits(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

type usageError string

func (value usageError) Error() string { return string(value) }

func writeClientError(stderr io.Writer, err error) int {
	switch {
	case errors.As(err, new(usageError)):
		writeUsageError(stderr, err)
		return ExitUsage
	case errors.Is(err, localapi.ErrUnavailable):
		fmt.Fprintln(stderr, "agent_unavailable: start krasis-agent and retry")
		return ExitAgentUnavailable
	case errors.Is(err, localapi.ErrAuthentication):
		fmt.Fprintln(stderr, "authentication_error: local agent credentials are invalid")
		return ExitAuthentication
	case errors.Is(err, localapi.ErrTransport), errors.Is(err, localapi.ErrProtocol):
		fmt.Fprintln(stderr, "transport_error: incompatible local agent response")
		return ExitTransport
	default:
		fmt.Fprintln(stderr, "internal_error: command failed")
		return ExitInternal
	}
}

func writeUsageError(stderr io.Writer, err error) {
	if err != nil {
		fmt.Fprintln(stderr, "usage_error:", err)
	}
	fmt.Fprintln(stderr, "usage: krasis-cli [--json] [--control-dir DIR] help|version|doctor|update|clipboard|status|diagnostics|topology|credential|connection|connect|disconnect|pair|trust|safe-stop")
}

func writeHuman(output io.Writer, value any) {
	switch item := value.(type) {
	case helpResult:
		fmt.Fprintln(output, "Krasis CLI commands:")
		for _, command := range item.Commands {
			fmt.Fprintf(output, "  %-55s %s\n", command.Usage, command.Summary)
		}
		fmt.Fprintln(output, "Use --json for agent and state-change flags.")
	case localapi.CredentialResult:
		fmt.Fprintf(output, "%s: %s\n", item.Status, item.Code)
		if item.Info != nil {
			fmt.Fprintf(output, "Role: %s\nFingerprint: %s\nExpires: %v\nExpired: %t\n", item.Info.Role, item.Info.Fingerprint, item.Info.NotAfter, item.Info.Expired)
		}
		if item.Code == "credential_renewed_repair_required" {
			fmt.Fprintln(output, "Re-pair with each peer before connecting; remote trust was not changed.")
		}
	case versionResult:
		fmt.Fprintf(output, "CLI: g%d %s\nAgent: %s", item.CLIGeneration, item.CLIBuild, item.AgentState)
		if item.AgentState == "RUNNING" {
			fmt.Fprintf(output, " g%d %s (protocol %d)", item.AgentGeneration, item.AgentBuild, item.AgentProtocol)
		}
		fmt.Fprintln(output)
	case doctorResult:
		fmt.Fprintf(output, "Agent: %s\nConnection: %s\nCLI: g%d\nAgent version: g%d\nProtocol: %s\n", item.Agent, item.Connection, item.CLIGeneration, item.AgentGeneration, item.Protocol)
		if item.PeerAlias != "" {
			fmt.Fprintf(output, "Peer: %s\n", item.PeerAlias)
		}
	case updateStatusResult:
		fmt.Fprintf(output, "Local: g%d %s\nPeer %s: %s", item.LocalGeneration, item.LocalBuild, item.PeerAlias, item.PeerState)
		if item.PeerState == "ONLINE" {
			fmt.Fprintf(output, " g%d %s", item.PeerGeneration, item.PeerBuild)
		}
		fmt.Fprintf(output, "\nPeer permits updates: %t\nReceive allowed: %t\nTransfer: %s\n", item.PeerAllowsUpdate, item.ReceiveAllowed, item.TransferState)
	case updatePermissionResult:
		state := "unknown"
		if item.ReceiveAllowed != nil {
			state = fmt.Sprint(*item.ReceiveAllowed)
		}
		fmt.Fprintf(output, "Receive updates from %s: %s (%s)\n", item.PeerAlias, state, item.Code)
	case updateVerifyResult:
		fmt.Fprintf(output, "Update bundle: %s", item.Code)
		if item.Valid {
			fmt.Fprintf(output, " (g%d %s, %d bytes)", item.Generation, item.Revision, item.SizeBytes)
		}
		fmt.Fprintln(output)
	case localapi.UpdateResult:
		fmt.Fprintf(output, "Update: %s\nTransfer: %s\n", item.Code, item.Status.State)
	case clipboardState:
		fmt.Fprintf(output, "Clipboard %s: enabled=%t (%s)\n", item.PeerAlias, item.Enabled, item.Code)
		if item.OtherPeerActive {
			fmt.Fprintln(output, "Another peer has clipboard sharing enabled.")
		}
	case localapi.ConnectionResult:
		fmt.Fprintf(output, "%s: %s\n", item.Status, item.Code)
		if item.Profile != nil {
			fmt.Fprintf(output, "Address: %s\nLocal exit edge: %s\nFingerprint: %s\n", item.Profile.Address, item.Profile.Edge, item.Profile.Fingerprint)
		}
	case localapi.Status:
		fmt.Fprintf(output, "State: %s\n", item.State)
		if item.PeerAlias != "" {
			fmt.Fprintf(output, "Peer: %s\n", item.PeerAlias)
		}
		fmt.Fprintf(output, "Actions: %s\n", strings.Join(item.Actions, ", "))
	case localapi.Diagnostics:
		fmt.Fprintf(output, "State: %s\n", item.State)
		keys := make([]string, 0, len(item.Counters))
		for key := range item.Counters {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(output, "%s: %d\n", key, item.Counters[key])
		}
	case diagnosticsExportResult:
		fmt.Fprintf(output, "Diagnostics: %s\n", item.Code)
		if item.Path != "" {
			fmt.Fprintf(output, "File: %s\n", item.Path)
		}
	case diagnosticTransferResult:
		fmt.Fprintf(output, "Diagnostics: %s\n", item.Code)
		if item.PeerAlias != "" {
			fmt.Fprintf(output, "Peer: %s\n", item.PeerAlias)
		}
		if item.Transfer != "" {
			fmt.Fprintf(output, "Latest transfer: %s\n", item.Transfer)
		}
	case localapi.Trust:
		fmt.Fprintf(output, "Trust revision: %d\n", item.Revision)
		for _, peer := range item.Peers {
			fmt.Fprintf(output, "%s %s inbound=%t revoked=%t fingerprint=%s\n", peer.Alias, peer.Role, peer.AllowInboundControl, peer.Revoked, peer.Fingerprint)
		}
	case localapi.Topology:
		fmt.Fprintf(output, "State: %s\nLayout: %s\n", item.State, item.Layout)
		for _, peer := range item.Peers {
			fmt.Fprintf(output, "%s connected=%t inbound=%t\n", peer.Alias, peer.Connected, peer.AllowInboundControl)
		}
	case localapi.OperationResult:
		fmt.Fprintf(output, "%s: %s\n", item.Status, item.Code)
		if item.PairingID != "" {
			fmt.Fprintf(output, "Pairing ID: %s\n", item.PairingID)
		}
		if item.Confirmation != "" {
			fmt.Fprintf(output, "Confirmation code: %s\n", item.Confirmation)
		}
		switch item.Code {
		case "pairing_completed":
			fmt.Fprintln(output, "Local trust commit succeeded; remote outcome is unknown. Check pair status on both devices before connecting.")
		case "pairing_commit_uncertain":
			fmt.Fprintln(output, "Local trust commit outcome is uncertain; remote outcome is unknown. Do not connect or repeat pairing yet. Check pair status and trust list on both devices.")
		case "pairing_failed", "pairing_expired", "pairing_cancelled":
			fmt.Fprintln(output, "This job did not attempt a local trust commit; existing trust may still exist and the remote outcome is unknown. Check pair status and trust list on both devices before retrying.")
		case "confirmation_submitted", "confirmation_already_submitted":
			fmt.Fprintln(output, "Confirmation was submitted, not completed. Query pair status with the same local job ID; do not start a new pairing job.")
		}
	}
}
