package inputworker

// A sampled holder at failed gate admission, not proof of the entire wait's
// cause. Contains only closed categories; it never authorizes retry or input.
type sourceGateFailure struct {
	operation string
	cause     error
}

func (e *sourceGateFailure) Error() string { return "source writer gate admission failed" }
func (e *sourceGateFailure) Unwrap() error { return e.cause }

func sourceGateOperation(state uint32) string {
	origin := sourceWriteOrigin(byte(state >> 8))
	stages := [...]string{"", "prepare", "presence", "write"}
	stage := byte(state)
	if origin == "" || stage == 0 || int(stage) >= len(stages) {
		return "source-gate-unobserved"
	}
	return "source-gate-" + origin + "-" + stages[stage]
}
