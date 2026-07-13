package labcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/truststore"
)

type trustImportArgs struct {
	storeDirectory      string
	peerCertificate     string
	alias               string
	allowInboundControl bool
}

type trustListArgs struct {
	storeDirectory string
	jsonOutput     bool
}

type trustRevokeArgs struct {
	storeDirectory string
	fingerprint    string
}

func runTrust(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeTrustUsage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "import":
		return runTrustImport(args[1:], stdout, stderr)
	case "list":
		return runTrustList(args[1:], stdout, stderr)
	case "revoke":
		return runTrustRevoke(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown trust command: %s\n", args[0])
		writeTrustUsage(stderr)
		return exitUsage
	}
}

func runTrustImport(args []string, stdout, stderr io.Writer) int {
	options, err := parseTrustImportArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid trust import options: %v\n", err)
		writeTrustUsage(stderr)
		return exitUsage
	}
	peer, err := peercredentials.LoadTrustedPeer(options.peerCertificate)
	if err != nil {
		return writeError(stderr, err)
	}
	store, err := truststore.New(options.storeDirectory)
	if err != nil {
		return writeError(stderr, err)
	}
	record, err := store.Import(peer, options.alias, options.allowInboundControl, time.Now())
	if err != nil {
		return writeError(stderr, err)
	}
	fmt.Fprintf(stdout, "Trusted peer imported: alias=%q role=%s fingerprint=%s inbound=%t\n",
		record.Alias, record.Role, record.Fingerprint, record.AllowInboundControl)
	return exitSuccess
}

func runTrustList(args []string, stdout, stderr io.Writer) int {
	options, err := parseTrustListArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid trust list options: %v\n", err)
		writeTrustUsage(stderr)
		return exitUsage
	}
	store, err := truststore.New(options.storeDirectory)
	if err != nil {
		return writeError(stderr, err)
	}
	document, err := store.List()
	if err != nil {
		return writeError(stderr, err)
	}
	if options.jsonOutput {
		if err := json.NewEncoder(stdout).Encode(document); err != nil {
			fmt.Fprintf(stderr, "output_error: %v\n", err)
			return exitRuntime
		}
		return exitSuccess
	}
	fmt.Fprintf(stdout, "Trust store: revision=%d updated=%s peers=%d\n",
		document.Revision, document.UpdatedAt.Format(time.RFC3339), len(document.Peers))
	for _, record := range document.Peers {
		fmt.Fprintf(stdout, "alias=%q role=%s fingerprint=%s inbound=%t revoked=%t\n",
			record.Alias, record.Role, record.Fingerprint, record.AllowInboundControl, record.RevokedAt != nil)
	}
	return exitSuccess
}

func runTrustRevoke(args []string, stdout, stderr io.Writer) int {
	options, err := parseTrustRevokeArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid trust revoke options: %v\n", err)
		writeTrustUsage(stderr)
		return exitUsage
	}
	store, err := truststore.New(options.storeDirectory)
	if err != nil {
		return writeError(stderr, err)
	}
	record, err := store.Revoke(options.fingerprint, time.Now())
	if err != nil {
		return writeError(stderr, err)
	}
	fmt.Fprintf(stdout, "Trusted peer revoked: alias=%q fingerprint=%s\n", record.Alias, record.Fingerprint)
	return exitSuccess
}

func parseTrustImportArgs(args []string) (trustImportArgs, error) {
	var parsed trustImportArgs
	values := map[string]*string{
		"--store": &parsed.storeDirectory, "--peer-cert": &parsed.peerCertificate, "--alias": &parsed.alias,
	}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--allow-inbound-control" {
			if parsed.allowInboundControl {
				return trustImportArgs{}, errors.New("--allow-inbound-control may be specified only once")
			}
			parsed.allowInboundControl = true
			continue
		}
		name, value, inline := strings.Cut(argument, "=")
		target, known := values[name]
		if !known {
			return trustImportArgs{}, fmt.Errorf("unknown option %q", argument)
		}
		if !inline {
			if index+1 >= len(args) {
				return trustImportArgs{}, fmt.Errorf("%s requires a value", name)
			}
			index++
			value = args[index]
		}
		if value == "" || *target != "" {
			return trustImportArgs{}, fmt.Errorf("%s requires one non-empty value", name)
		}
		*target = value
	}
	if parsed.storeDirectory == "" || parsed.peerCertificate == "" || parsed.alias == "" || !parsed.allowInboundControl {
		return trustImportArgs{}, errors.New("--store, --peer-cert, --alias, and --allow-inbound-control are required")
	}
	return parsed, nil
}

func parseTrustListArgs(args []string) (trustListArgs, error) {
	var parsed trustListArgs
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--json" {
			if parsed.jsonOutput {
				return trustListArgs{}, errors.New("--json may be specified only once")
			}
			parsed.jsonOutput = true
			continue
		}
		name, value, inline := strings.Cut(argument, "=")
		if name != "--store" || parsed.storeDirectory != "" {
			return trustListArgs{}, fmt.Errorf("unknown or duplicate option %q", argument)
		}
		if !inline {
			if index+1 >= len(args) {
				return trustListArgs{}, errors.New("--store requires a value")
			}
			index++
			value = args[index]
		}
		parsed.storeDirectory = value
	}
	if parsed.storeDirectory == "" {
		return trustListArgs{}, errors.New("--store is required")
	}
	return parsed, nil
}

func parseTrustRevokeArgs(args []string) (trustRevokeArgs, error) {
	var parsed trustRevokeArgs
	values := map[string]*string{"--store": &parsed.storeDirectory, "--fingerprint": &parsed.fingerprint}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		name, value, inline := strings.Cut(argument, "=")
		target, known := values[name]
		if !known || *target != "" {
			return trustRevokeArgs{}, fmt.Errorf("unknown or duplicate option %q", argument)
		}
		if !inline {
			if index+1 >= len(args) {
				return trustRevokeArgs{}, fmt.Errorf("%s requires a value", name)
			}
			index++
			value = args[index]
		}
		*target = value
	}
	if parsed.storeDirectory == "" || parsed.fingerprint == "" {
		return trustRevokeArgs{}, errors.New("--store and --fingerprint are required")
	}
	return parsed, nil
}

func writeTrustUsage(output io.Writer) {
	fmt.Fprintln(output, "usage:")
	fmt.Fprintln(output, "  krasis-lab trust import --store DIRECTORY --peer-cert PEER.pem --alias NAME --allow-inbound-control")
	fmt.Fprintln(output, "  krasis-lab trust list --store DIRECTORY [--json]")
	fmt.Fprintln(output, "  krasis-lab trust revoke --store DIRECTORY --fingerprint SHA256")
}
