package logindiag

import "strings"

var sourceQueryDeadlinePhases = [...]string{"admission", "waiting", "scheduled", "checking", "publishing", "retiring", "completed"}

func SourceQueryDeadlineOperations() []string {
	var operations []string
	for _, owner := range []string{"", "service-", "agent-"} {
		for _, phase := range sourceQueryDeadlinePhases {
			operations = append(operations, "source-"+owner+"query-deadline-"+phase)
		}
	}
	return operations
}

func ValidSourceQueryDeadlineOperation(operation string) bool {
	for _, prefix := range []string{"source-query-deadline-", "source-service-query-deadline-", "source-agent-query-deadline-"} {
		if phase, ok := strings.CutPrefix(operation, prefix); ok {
			return member(phase, sourceQueryDeadlinePhases[:]...)
		}
	}
	return false
}
