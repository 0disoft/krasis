package logindiag

import "testing"

func TestInputRetirementOperationsAreClosedAndLegacyCompatible(t *testing.T) {
	for _, phase := range []string{"bootstrap", "native"} {
		for _, kind := range []string{"other", "pending", "timeout", "cancelled", "truncated", "eof", "closed", "win32"} {
			if !ValidRuntimeOperation("input-retire-" + phase + "-" + kind) {
				t.Fatal("bounded child failure rejected", phase, kind)
			}
		}
	}
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
		"input-retire-native-SECRET", "input-retire-bootstrap-win32-extra",
	} {
		if ValidRuntimeOperation(value) {
			t.Fatal("unbounded or inapplicable diagnostic code accepted", value)
		}
	}
}
