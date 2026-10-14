package inputworker

// Fixed worker-local phase evidence, not a retry instruction or input payload.
type failureStage byte

const (
	failureStageUnknown failureStage = iota
	failureStageBeforeDispatch
	failureStageApply
	failureStageCursorReadback
	failureStageExecutorCompletion
)

func (stage failureStage) valid() bool {
	return stage >= failureStageBeforeDispatch && stage <= failureStageExecutorCompletion
}

func (stage failureStage) code() string {
	switch stage {
	case failureStageBeforeDispatch:
		return "installed_input_failure_scope_admission"
	case failureStageApply:
		return "installed_input_failure_apply"
	case failureStageCursorReadback:
		return "installed_input_failure_cursor_readback"
	case failureStageExecutorCompletion:
		return "installed_input_failure_scope_completion"
	default:
		return ""
	}
}
