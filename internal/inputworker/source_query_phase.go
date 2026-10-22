package inputworker

import "context"

const (
	queryScheduled uint32 = iota
	queryChecking
	queryPublishing
	queryRetiring
	queryCompleted
)

func (q *authorityQuery) phaseName() string {
	return [...]string{"scheduled", "checking", "publishing", "retiring", "completed"}[q.phase.Load()]
}

func queryPhaseFailure(kind authorityQueryKind, phase string, cause error) error {
	failure := queryAdmissionFailure(kind, "", cause)
	if query, ok := failure.(*sourceQueryFailure); ok && cause == context.DeadlineExceeded {
		query.reason += "-" + phase
	}
	return failure
}
