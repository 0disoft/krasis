package logindiag

import "testing"

func TestInputRetirementOperationsAreClosedAndLegacyCompatible(t *testing.T) {
	for _, stage := range []string{"authentication", "stop", "receipt"} {
		for _, suffix := range []string{"", "-timeout", "-cancelled", "-truncated", "-eof", "-closed"} {
			if !ValidRuntimeOperation("input-retire-" + stage + suffix) {
				t.Fatal("retirement transport class rejected", stage, suffix)
			}
		}
	}
	for _, value := range []string{
		"input-retire-validation-timeout", "input-retire-deadline-eof",
		"input-retire-receipt-SECRET", "input-retire-receipt-eof/path",
		"input-retire-receipt-timeout-extra", "input-retire-ready-timeout",
		"input-startup-receipt-eof", "input-retire-SECRET-eof",
	} {
		if ValidRuntimeOperation(value) {
			t.Fatal("unbounded or inapplicable diagnostic code accepted", value)
		}
	}
}
