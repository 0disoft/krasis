package productcli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestHelpIsOfflineAndMarksStateChanges(t *testing.T) {
	deps := dependencies{openClient: func(string) (apiClient, error) {
		t.Fatal("help opened the agent")
		return nil, nil
	}}
	for _, args := range [][]string{{"help", "--json"}, {"--help", "--json"}, {"-h", "--json"}} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr, deps); code != ExitOK || stderr.Len() != 0 {
			t.Fatalf("help %v: code=%d stderr=%q", args, code, stderr.String())
		}
		var result helpResult
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.SchemaVersion != helpSchema || len(result.Commands) < 25 {
			t.Fatalf("incomplete command catalog: %+v", result)
		}
		seen := map[string]bool{}
		for _, command := range result.Commands {
			if command.Usage == "" || command.Summary == "" || seen[command.Usage] {
				t.Fatalf("invalid or duplicate help entry: %+v", command)
			}
			seen[command.Usage] = true
			switch command.Usage {
			case "status":
				if !command.RequiresAgent || command.ChangesState {
					t.Fatalf("status effect: %+v", command)
				}
			case "diagnostics export":
				if command.RequiresAgent || !command.ChangesState {
					t.Fatalf("export effect: %+v", command)
				}
			case "update send ALIAS":
				if !command.RequiresAgent || !command.ChangesState {
					t.Fatalf("send effect: %+v", command)
				}
			}
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"help", "--control-dir", `C:\unused`}, &stdout, &stderr, deps); code != ExitUsage || stdout.Len() != 0 || !strings.Contains(stderr.String(), "help does not use") {
		t.Fatalf("unexpected control override: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
