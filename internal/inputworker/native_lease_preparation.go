package inputworker

// Local installed-code policy, not a readiness diagnostic or peer capability.
// Only ordinary installed input uses the independently authenticated pair.
func nativeLeasePreparation(installed bool, lease Lease) bool {
	return installed && !lease.Capture && !lease.Presentation
}
