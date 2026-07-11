package labcli

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/lab"
)

func TestParseAgentSessionConnectArgs(t *testing.T) {
	options, err := parseAgentSessionArgs([]string{
		"connect", "--device", "H-B", "--address", "192.0.2.10:24876", "--server-name", "192.0.2.10",
		"--ca", "ca.pem", "--cert", "identity.pem", "--key", "identity-key.pem",
		"--edge", "left", "--duration", "2m", "--output", "report.json", "--allow-live-input",
	})
	if err != nil {
		t.Fatalf("parseAgentSessionArgs() error = %v", err)
	}
	if options.edge != control.ScreenEdgeLeft || options.duration != 2*time.Minute || options.rearmDistance != defaultAgentRearmDistance {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseAgentSessionRequiresLiveInputAcknowledgement(t *testing.T) {
	_, err := parseAgentSessionArgs([]string{
		"listen", "--device", "H-A", "--address", ":24876", "--ca", "ca.pem",
		"--cert", "identity.pem", "--key", "identity-key.pem", "--edge", "RIGHT",
		"--duration", "1m", "--output", "report.json",
	})
	if err == nil {
		t.Fatal("parseAgentSessionArgs() accepted missing acknowledgement")
	}
}

func TestAgentSessionEvidenceRequiresConnectionAndDestinationInput(t *testing.T) {
	report := lab.AgentSessionReport{}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "no_authenticated_session" {
		t.Fatalf("no-session evidence = code %d report %+v", code, report)
	}
	report = lab.AgentSessionReport{Supervisor: lab.SupervisorEvidence{ActiveCount: 1}}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "no_destination_input_observed" {
		t.Fatalf("no-input evidence = code %d report %+v", code, report)
	}
	report = lab.AgentSessionReport{
		Supervisor:       lab.SupervisorEvidence{ActiveCount: 1},
		DestinationInput: lab.InputEvidence{Entries: 1, PointerMoves: 1},
	}
	if code := agentSessionEvidenceExit(&report); code != exitSuccess || !report.Passed {
		t.Fatalf("passing evidence = code %d report %+v", code, report)
	}
}
