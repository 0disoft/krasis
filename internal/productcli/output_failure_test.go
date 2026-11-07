package productcli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type failedOutput struct{ short bool }

func (w failedOutput) Write(p []byte) (int, error) {
	if w.short {
		return 0, nil
	}
	return 0, errors.New("write failed")
}

func TestOutputFailureReturnsInternalError(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		for _, short := range []bool{false, true} {
			var stderr bytes.Buffer
			code := writeOutput(failedOutput{short: short}, &stderr, commandHelp(), ExitOK, jsonOutput)
			if code != ExitInternal || !strings.Contains(stderr.String(), "internal_error: write output") {
				t.Fatalf("json=%t short=%t: exit=%d stderr=%q", jsonOutput, short, code, stderr.String())
			}
		}
	}
}
