package labcli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
)

type peerCredentialsArgs struct {
	outputDirectory string
	serverName      string
}

type peerCredentialsProtectArgs struct {
	directory string
}

func runPeerCredentials(args []string, stdout, stderr io.Writer) int {
	options, err := parsePeerCredentialsArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid peer-credentials options: %v\n", err)
		fmt.Fprintln(stderr, "usage: krasis-lab peer-credentials --output DIRECTORY --server-name NAME")
		return exitUsage
	}
	manifest, err := peercredentials.Generate(options.outputDirectory, options.serverName, time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "runtime_error: %s\n", strings.TrimSpace(err.Error()))
		return exitRuntime
	}
	fmt.Fprintf(stdout, "Peer credential bundle: %s\n", options.outputDirectory)
	fmt.Fprintf(stdout, "Expires: %s\n", manifest.ExpiresAt.Format(time.RFC3339))
	fmt.Fprintln(stdout, "Copy only listener/ to the listener and only connector/ to the connector.")
	return exitSuccess
}

func runPeerCredentialsProtect(args []string, stdout, stderr io.Writer) int {
	options, err := parsePeerCredentialsProtectArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid peer-credentials-protect options: %v\n", err)
		fmt.Fprintln(stderr, "usage: krasis-lab peer-credentials-protect --directory ROLE_DIRECTORY")
		return exitUsage
	}
	if err := peercredentials.ProtectPrivateKeyDirectory(options.directory); err != nil {
		fmt.Fprintf(stderr, "runtime_error: %s\n", strings.TrimSpace(err.Error()))
		return exitRuntime
	}
	fmt.Fprintf(stdout, "Protected peer credential directory: %s\n", options.directory)
	return exitSuccess
}

func parsePeerCredentialsArgs(args []string) (peerCredentialsArgs, error) {
	var parsed peerCredentialsArgs
	values := map[string]*string{"--output": &parsed.outputDirectory, "--server-name": &parsed.serverName}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		name, value, inline := strings.Cut(argument, "=")
		target, known := values[name]
		if !known {
			return peerCredentialsArgs{}, fmt.Errorf("unknown option %q", argument)
		}
		if !inline {
			if index+1 >= len(args) {
				return peerCredentialsArgs{}, fmt.Errorf("%s requires a value", name)
			}
			index++
			value = args[index]
		}
		if value == "" {
			return peerCredentialsArgs{}, fmt.Errorf("%s requires a value", name)
		}
		if *target != "" {
			return peerCredentialsArgs{}, fmt.Errorf("%s may be specified only once", name)
		}
		*target = value
	}
	if parsed.outputDirectory == "" || parsed.serverName == "" {
		return peerCredentialsArgs{}, errors.New("--output and --server-name are required")
	}
	return parsed, nil
}

func parsePeerCredentialsProtectArgs(args []string) (peerCredentialsProtectArgs, error) {
	var parsed peerCredentialsProtectArgs
	for index := 0; index < len(args); index++ {
		argument := args[index]
		name, value, inline := strings.Cut(argument, "=")
		if name != "--directory" {
			return peerCredentialsProtectArgs{}, fmt.Errorf("unknown option %q", argument)
		}
		if !inline {
			if index+1 >= len(args) {
				return peerCredentialsProtectArgs{}, errors.New("--directory requires a value")
			}
			index++
			value = args[index]
		}
		if value == "" {
			return peerCredentialsProtectArgs{}, errors.New("--directory requires a value")
		}
		if parsed.directory != "" {
			return peerCredentialsProtectArgs{}, errors.New("--directory may be specified only once")
		}
		parsed.directory = value
	}
	if parsed.directory == "" {
		return peerCredentialsProtectArgs{}, errors.New("--directory is required")
	}
	return parsed, nil
}
