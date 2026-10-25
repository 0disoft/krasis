package logindiag

import "strings"

// Service-local preparation/retirement stages, never caller text or numeric
// child report codes. Existing support fields and report framing stay intact.
func validInputServiceOperation(operation string) bool {
	if phase, kind, ok := strings.Cut(strings.TrimPrefix(operation, "input-retire-"), "-"); ok && strings.HasPrefix(operation, "input-retire-") && (phase == "bootstrap" || phase == "native") {
		switch kind {
		case "other", "pending", "timeout", "cancelled", "truncated", "eof", "closed", "win32":
			return true
		}
	}
	switch operation {
	case "input-startup-validation", "input-startup-prepare", "input-startup-authentication", "input-startup-transfer", "input-startup-ready", "input-startup-publish",
		"input-prepare-launch", "input-prepare-connect", "input-prepare-permit",
		"input-retire-validation", "input-retire-deadline", "input-retire-authentication", "input-retire-stop", "input-retire-receipt",
		"input-close-control", "input-close-worker":
		return true
	}
	prefix := "input-startup-"
	if strings.HasPrefix(operation, "input-retire-") {
		prefix = "input-retire-"
	}
	stage, kind, ok := strings.Cut(strings.TrimPrefix(operation, prefix), "-")
	if !ok || !strings.HasPrefix(operation, prefix) {
		return false
	}
	validStage := false
	if prefix == "input-retire-" {
		validStage = stage == "authentication" || stage == "stop" || stage == "receipt"
	} else {
		switch stage {
		case "validation", "prepare", "authentication", "transfer", "ready", "publish":
			validStage = true
		}
	}
	if validStage {
		switch kind {
		case "timeout", "cancelled", "truncated", "eof", "closed":
			return true
		}
	}
	return false
}
