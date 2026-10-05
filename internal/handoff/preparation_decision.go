package handoff

// PreparationDecision is service-owned diagnostic data, never input authority.
// Zero preserves an unobserved decision in older readiness receipts.
type PreparationDecision uint8

const (
	PreparationUnobserved PreparationDecision = iota
	PreparationUsed
	PreparationNoCandidate
	PreparationExpired
	PreparationExited
	PreparationSessionChanged
	PreparationClaimLost
	PreparationCancelled
	PreparationUnusable
	PreparationInvalidAge
)

func validPreparationDecision(p StartupProfile) bool {
	if p.PreparationDecision > PreparationInvalidAge {
		return false
	}
	if p.PreparationDecision == PreparationUnobserved {
		return true
	}
	return p.Measured&3 == 3 && (p.PreparationDecision == PreparationUsed) == p.Prepared
}
