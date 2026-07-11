package labcli

import (
	"fmt"
	"io"
	"strings"

	"github.com/0disoft/krasis/internal/phase0report"
)

func runPhase0Check(args []string, stdout, stderr io.Writer) int {
	jsonOutput := false
	home := make([]string, 0, 2)
	study := make([]string, 0, 2)
	recoveries := make([]string, 0, 4)
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--json" {
			if jsonOutput {
				fmt.Fprintln(stderr, "invalid phase0-check options: --json may be specified only once")
				return exitUsage
			}
			jsonOutput = true
			continue
		}
		if argument != "--home" && argument != "--study" && argument != "--recovery" {
			fmt.Fprintf(stderr, "invalid phase0-check options: unknown option %q\n", argument)
			return exitUsage
		}
		if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
			fmt.Fprintf(stderr, "invalid phase0-check options: %s requires a report path\n", argument)
			return exitUsage
		}
		index++
		switch argument {
		case "--home":
			home = append(home, args[index])
		case "--study":
			study = append(study, args[index])
		case "--recovery":
			recoveries = append(recoveries, args[index])
		}
	}
	if len(home) != 2 || len(study) != 2 || len(recoveries) != 4 {
		writePhase0CheckUsage(stderr)
		return exitUsage
	}
	result := phase0report.Check(home, study, recoveries)
	if jsonOutput {
		if code := writeJSON(stdout, stderr, result); code != exitSuccess {
			return code
		}
	} else {
		fmt.Fprintf(stdout, "Phase 0 evidence: passed=%t home=%t study=%t recoveries=%d\n", result.Passed, result.Home.PairPassed, result.Study.PairPassed, len(result.Recoveries))
		if len(result.Issues) > 0 {
			fmt.Fprintf(stdout, "Issues: %s\n", strings.Join(result.Issues, ","))
		}
	}
	if !result.Passed {
		return exitRuntime
	}
	return exitSuccess
}

func writePhase0CheckUsage(output io.Writer) {
	fmt.Fprintln(output, "usage: krasis-lab phase0-check [--json] --home H-A.json --home H-B.json --study S-A.json --study S-B.json --recovery H-A.json --recovery H-B.json --recovery S-A.json --recovery S-B.json")
}
