package inputworker

import "testing"

func TestColdNativePreparationOnlyForOrdinaryInstalledLease(t *testing.T) {
	for _, installed := range []bool{false, true} {
		for _, lease := range []Lease{{}, {Capture: true}, {Presentation: true}, {Capture: true, Presentation: true}} {
			if got := nativeLeasePreparation(installed, lease); got != (installed && !lease.Capture && !lease.Presentation) {
				t.Fatal("native pair escaped installed ordinary-input policy", installed, lease, got)
			}
		}
	}
}
