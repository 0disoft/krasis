package logindiag

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
	return false
}
