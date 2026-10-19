package inputworker

import (
	"bytes"
	"errors"
	"io"

	"github.com/0disoft/krasis/internal/handoff"
)

// Only the authenticated source child may finish before native readiness.
// A final report is not readiness: preserve it for the service's cleanup fence
// and diagnostics without publishing an input/capture-ready receipt.
func readSourceWorkerStartup(reader io.Reader) (*handoff.StartupProfile, Report, bool, error) {
	var prefix [8]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		_, failure := receipt(Report{}, &workerReportFailure{operation: "worker-report-read", cause: errors.Join(ErrProtocol, err)})
		return nil, Report{}, false, failure
	}
	reader = io.MultiReader(bytes.NewReader(prefix[:]), reader)
	if string(prefix[:4]) != "KRR1" {
		profile, err := readStartupReady(reader)
		if err != nil {
			_, err = receipt(Report{}, err)
		}
		return profile, Report{}, false, err
	}
	report, err := readReport(reader)
	report, err = receipt(report, err)
	if report.Source == nil && err == nil {
		err = ErrRecovery // An injection report cannot certify a source child.
	}
	return nil, report, true, err
}
