package productcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	control, err := ResolveControlDirectory(deps.localAppData, parsed.controlDir)
	if err != nil {
		fmt.Fprintln(stderr, "config_error: invalid CLI configuration")
		return ExitUsage
	}
	client, err := deps.openClient(control)
	if err != nil {
		return writeClientError(stderr, err)
	}
	value, code, err := dispatch(ctx, client, parsed.args)
	if err != nil {
		return writeClientError(stderr, err)
	}
	if parsed.json {
		if err := json.NewEncoder(stdout).Encode(value); err != nil {
			fmt.Fprintln(stderr, "internal_error: write output")
			return ExitInternal
		}
	} else {
		writeHuman(stdout, value)
	}
	return code
}

func dispatch(ctx context.Context, client apiClient, args []string) (any, int, error) {
	switch args[0] {
	case "connection":
		return dispatchConnection(ctx, client, args[1:])
	case "status":
		if len(args) != 1 {
			return nil, ExitUsage, usageError("status accepts no arguments")
		}
		value, err := client.Status(ctx)
		return value, ExitOK, err
	case "diagnostics":
		if len(args) != 1 {
			return nil, ExitUsage, usageError("diagnostics accepts no arguments")
		}
		value, err := client.Diagnostics(ctx)
		return value, ExitOK, err
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
	case len(args) == 3 && args[0] == "begin" && args[2] == "--allow-inbound-control":
		allow := true
		return operation(client.Pair(ctx, localapi.OperationRequest{PairingAction: "BEGIN", PeerAlias: args[1], AllowInboundControl: &allow}))
	case len(args) == 3 && args[0] == "confirm":
		return operation(client.Pair(ctx, localapi.OperationRequest{PairingAction: "CONFIRM", PairingID: args[1], ConfirmationCode: args[2]}))
	case len(args) == 2 && args[0] == "cancel":
		return operation(client.Pair(ctx, localapi.OperationRequest{PairingAction: "CANCEL", PairingID: args[1]}))
	default:
		return nil, ExitUsage, usageError("pair requires begin, confirm, or cancel arguments")
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
		case "--control-dir":
			if result.controlDir != "" || index+1 >= len(args) {
				return options{}, usageError("--control-dir requires one value")
			}
			index++
			result.controlDir = args[index]
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
	case "connection":
		_, err := parseConnection(args[1:])
		return err
	case "status", "diagnostics", "topology", "disconnect", "safe-stop":
		if len(args) != 1 {
			return usageError(args[0] + " accepts no arguments")
		}
	case "connect":
		if len(args) != 2 || !validAlias(args[1]) {
			return usageError("connect requires a valid peer alias")
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
		case len(args) == 4 && args[1] == "begin" && validAlias(args[2]) && args[3] == "--allow-inbound-control":
		case len(args) == 4 && args[1] == "confirm" && validHex(args[2], 32) && validDigits(args[3], 6):
		case len(args) == 3 && args[1] == "cancel" && validHex(args[2], 32):
		default:
			return usageError("pair requires valid begin, confirm, or cancel arguments")
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
	fmt.Fprintln(stderr, "usage: krasis [--json] [--control-dir DIR] status|diagnostics|topology|connection|connect|disconnect|pair|trust|safe-stop")
}

func writeHuman(output io.Writer, value any) {
	switch item := value.(type) {
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
	}
}
