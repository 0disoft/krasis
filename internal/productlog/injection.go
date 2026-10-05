package productlog

import (
	"errors"
	"github.com/0disoft/krasis/internal/application"
)

// Counts describe API insertion only, not successful delivery to a target app.
// No keys, coordinates, target window names or inferred UIPI cause are recorded.
type InjectionFailure struct {
	Operation   string                         `json:"operation"`
	Certainty   application.InjectionCertainty `json:"certainty"`
	Requested   int                            `json:"requested"`
	Inserted    int                            `json:"inserted"`
	CountsKnown bool                           `json:"countsKnown"`
	CursorStage string                         `json:"cursorStage,omitempty"`
}

func init() { allowed["injection_failure"] = true }

func (s InjectionFailure) valid() bool {
	if s.CursorStage != "" && !application.ValidCursorFailureStage(s.Operation, s.CursorStage) {
		return false
	}
	switch s.Operation {
	case "press", "release", "move", "wheel", "entry", "rollback":
	default:
		return false
	}
	switch s.Certainty {
	case application.InjectionNotApplied, application.InjectionPartiallyApplied, application.InjectionOutcomeUnknown:
	default:
		return false
	}
	if !s.CountsKnown {
		return s.Requested == 0 && s.Inserted == 0 && s.Certainty == application.InjectionOutcomeUnknown
	}
	if s.Requested <= 0 || s.Requested > 65536 || s.Inserted < 0 || s.Inserted > s.Requested {
		return false
	}
	switch s.Certainty {
	case application.InjectionNotApplied:
		return s.Inserted == 0
	case application.InjectionPartiallyApplied:
		return s.Inserted > 0 && s.Inserted < s.Requested
	default:
		return true
	}
}

func (l *Logger) Injection(operation string, err error) {
	if err == nil {
		return
	}
	s := InjectionFailure{Operation: operation, Certainty: application.InjectionOutcomeUnknown}
	var failure *application.InjectionError
	if errors.As(err, &failure) && failure != nil {
		candidate := InjectionFailure{Operation: operation, Certainty: failure.Certainty, Requested: failure.Requested, Inserted: failure.Inserted, CountsKnown: true}
		if candidate.valid() {
			s = candidate
		}
	}
	var cursor *application.CursorTransactionFailure
	if errors.As(err, &cursor) && cursor != nil && cursor.Cause != nil && cursor.Operation == operation && application.ValidCursorFailureStage(operation, cursor.Stage) {
		s.CursorStage = cursor.Stage
	}
	if s.valid() {
		l.enqueueRecord(record{Code: "injection_failure", Injection: &s})
	}
}
