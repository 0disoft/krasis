package logindiag

import "strings"

// Closed categories only: never action kinds, coordinates, payloads or paths.
var sourceWriteOrigins = [...]string{"boundary", "action", "reply", "return", "retirement"}
var sourceWriteReasons = [...]string{"deadline", "unsubmitted", "partial", "timeout", "closed", "native", "other"}

func SourceWriteOriginOperations() []string {
	operations := make([]string, 0, len(sourceWriteOrigins)*len(sourceWriteReasons))
	for _, origin := range sourceWriteOrigins {
		for _, reason := range sourceWriteReasons {
			operations = append(operations, "source-write-"+origin+"-"+reason)
		}
	}
	return operations
}

func validSourceWriteOriginOperation(operation string) bool {
	value, ok := strings.CutPrefix(operation, "source-write-")
	if !ok {
		return false
	}
	origin, reason, ok := strings.Cut(value, "-")
	return ok && member(origin, sourceWriteOrigins[:]...) && member(reason, sourceWriteReasons[:]...)
}
