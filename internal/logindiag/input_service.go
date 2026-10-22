package logindiag

import "strings"

// Service-local preparation/retirement stages, never caller text or numeric
// child report codes. Existing support fields and report framing stay intact.
func validInputServiceOperation(operation string) bool {
	switch operation {
	case "input-startup-validation", "input-startup-prepare", "input-startup-authentication", "input-startup-transfer", "input-startup-ready", "input-startup-publish",
		"input-prepare-launch", "input-prepare-connect", "input-prepare-permit",
		"input-retire-validation", "input-retire-deadline", "input-retire-authentication", "input-retire-stop", "input-retire-receipt",
		"input-close-control", "input-close-worker":
		return true
	}
	stage, kind, ok := strings.Cut(strings.TrimPrefix(operation, "input-startup-"), "-")
	if !ok || !strings.HasPrefix(operation, "input-startup-") {
		return false
	}
	switch stage {
	case "validation", "prepare", "authentication", "transfer", "ready", "publish":
		switch kind {
		case "timeout", "cancelled", "truncated", "eof", "closed":
			return true
		}
	}
	return false
}
