package labcli

import (
	"fmt"
	"io"
	"strings"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/guardreport"
)

func runCrashRecoveryCheck(args []string, stdout, stderr io.Writer) int {
	jsonOutput := false
	paths := make([]string, 0, 1)
	for _, argument := range args {
		switch {
		case argument == "--json":
			if jsonOutput {
				fmt.Fprintln(stderr, "invalid crash-recovery-check options: --json may be specified only once")
				return exitUsage
			}
			jsonOutput = true
		case strings.HasPrefix(argument, "-"):
			fmt.Fprintf(stderr, "invalid crash-recovery-check options: unknown option %q\n", argument)
			return exitUsage
		default:
			paths = append(paths, argument)
		}
	}
	if len(paths) != 1 {
		fmt.Fprintln(stderr, "usage: krasis-lab crash-recovery-check [--json] GUARD.json")
		return exitUsage
	}
	result := guardreport.Check(paths[0], guardreport.Expectation{
		Reason: crashguard.ExitParentEOF, RequireCleanBuild: true, RequireRecovery: true,
	})
	if jsonOutput {
		if code := writeJSON(stdout, stderr, result); code != exitSuccess {
			return code
		}
	} else {
		fmt.Fprintf(stdout, "Crash recovery: valid=%t device=%s attempted=%d released=%d remaining=%d\n", result.Valid, result.DeviceAlias, result.Attempted, result.Released, result.Remaining)
		if len(result.Issues) > 0 {
			fmt.Fprintf(stdout, "Issues: %s\n", strings.Join(result.Issues, ","))
		}
	}
	if !result.Valid {
		return exitRuntime
	}
	return exitSuccess
}
