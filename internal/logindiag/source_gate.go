package logindiag

import "strings"

var sourceGateStages = [...]string{"prepare", "presence", "write"}

func SourceGateOperations() []string {
	operations := []string{"source-gate-unobserved"}
	for _, origin := range sourceWriteOrigins {
		for _, stage := range sourceGateStages {
			operations = append(operations, "source-gate-"+origin+"-"+stage)
		}
	}
	return operations
}

func validSourceGateOperation(operation string) bool {
	if operation == "source-gate-unobserved" {
		return true
	}
	value, ok := strings.CutPrefix(operation, "source-gate-")
	if !ok {
		return false
	}
	origin, stage, ok := strings.Cut(value, "-")
	return ok && member(origin, sourceWriteOrigins[:]...) && member(stage, sourceGateStages[:]...)
}
