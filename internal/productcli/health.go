package productcli

import (
	"context"
	"runtime/debug"

	"github.com/0disoft/krasis/internal/localapi"
)

const (
	versionSchema = "krasis.cli.version/v1"
	doctorSchema  = "krasis.cli.doctor/v1"
)

type versionResult struct {
	SchemaVersion   string `json:"schemaVersion"`
	CLIGeneration   int    `json:"cliGeneration"`
	CLIBuild        string `json:"cliBuild"`
	AgentState      string `json:"agentState"`
	AgentGeneration int    `json:"agentGeneration,omitempty"`
	AgentBuild      string `json:"agentBuild,omitempty"`
	AgentProtocol   int    `json:"agentProtocol,omitempty"`
}

type doctorResult struct {
	SchemaVersion   string `json:"schemaVersion"`
	Agent           string `json:"agent"`
	Connection      string `json:"connection"`
	PeerAlias       string `json:"peerAlias,omitempty"`
	CLIGeneration   int    `json:"cliGeneration"`
	AgentGeneration int    `json:"agentGeneration"`
	Protocol        string `json:"protocol"`
}

func localVersion() versionResult {
	v := versionResult{SchemaVersion: versionSchema, CLIGeneration: localapi.AgentGeneration, CLIBuild: "development", AgentState: "UNAVAILABLE"}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				v.CLIBuild = setting.Value
			}
		}
	}
	return v
}

func version(ctx context.Context, client apiClient) versionResult {
	v := localVersion()
	if client == nil {
		return v
	}
	runtime, err := client.Runtime(ctx)
	if err != nil {
		v.AgentState = "UNKNOWN"
		return v
	}
	v.AgentState = "RUNNING"
	v.AgentGeneration = runtime.Generation
	v.AgentBuild = runtime.Build
	v.AgentProtocol = runtime.Protocol
	return v
}

func doctor(ctx context.Context, client apiClient) (doctorResult, int, error) {
	runtime, err := client.Runtime(ctx)
	if err != nil {
		return doctorResult{}, ExitInternal, err
	}
	status, err := client.Status(ctx)
	if err != nil {
		return doctorResult{}, ExitInternal, err
	}
	v := doctorResult{SchemaVersion: doctorSchema, Agent: "RUNNING", Connection: status.State, PeerAlias: status.PeerAlias, CLIGeneration: localapi.AgentGeneration, AgentGeneration: runtime.Generation, Protocol: "COMPATIBLE"}
	if runtime.Protocol != localapi.AgentProtocol {
		v.Protocol = "INCOMPATIBLE"
		return v, ExitTransport, nil
	}
	if status.State != "CONNECTED" {
		return v, ExitUnsafeState, nil
	}
	return v, ExitOK, nil
}
