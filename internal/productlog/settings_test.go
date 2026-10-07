package productlog

import "testing"

func TestSettingsRestorationEventsRemainFixedAndAllowed(t *testing.T) {
	for _, stage := range []string{"startup", "recovery"} {
		for _, result := range []string{"query_failed", "write_failed", "mismatch", "verified"} {
			if !allowed["settings_restore_"+stage+"_"+result] {
				t.Fatal("restoration evidence filtered out", stage, result)
			}
		}
	}
	if allowed["settings_restore_private-path_verified"] {
		t.Fatal("arbitrary restoration metadata allowed")
	}
}
