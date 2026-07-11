package labcli

import (
	"fmt"
	"io"
	"strings"

	"github.com/0disoft/krasis/internal/agentreport"
)

func runAgentSessionCheck(args []string, stdout, stderr io.Writer) int {
	jsonOutput := false
	paths := make([]string, 0, 2)
	for _, argument := range args {
		switch {
		case argument == "--json":
			if jsonOutput {
				fmt.Fprintln(stderr, "invalid agent-session-check options: --json may be specified only once")
				return exitUsage
			}
			jsonOutput = true
		case strings.HasPrefix(argument, "-"):
			fmt.Fprintf(stderr, "invalid agent-session-check options: unknown option %q\n", argument)
			return exitUsage
		default:
			paths = append(paths, argument)
		}
	}
	if len(paths) != 2 {
		fmt.Fprintln(stderr, "usage: krasis-lab agent-session-check [--json] REPORT-A.json REPORT-B.json")
		return exitUsage
	}
	report := agentreport.Check(paths)
	if jsonOutput {
		if code := writeJSON(stdout, stderr, report); code != exitSuccess {
			return code
		}
	} else {
		fmt.Fprintf(stdout, "Agent session pair: valid=%t passed=%t\n", report.Valid, report.PairPassed)
		for _, file := range report.Files {
			fmt.Fprintf(stdout, "  %s: valid=%t device=%s role=%s edge=%s passed=%t", file.File, file.Valid, file.DeviceAlias, file.Role, file.Edge, file.Passed)
			if len(file.Issues) > 0 {
				fmt.Fprintf(stdout, " issues=%s", strings.Join(file.Issues, ","))
			}
			fmt.Fprintln(stdout)
		}
		if len(report.Issues) > 0 {
			fmt.Fprintf(stdout, "Issues: %s\n", strings.Join(report.Issues, ","))
		}
	}
	if !report.PairPassed {
		return exitRuntime
	}
	return exitSuccess
}
