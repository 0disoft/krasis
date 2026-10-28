package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
)

// ErrStartupRetired is a rejection, never readiness or permission to inject.
// Only an authenticated service's complete, empty final report produces it.
var ErrStartupRetired = errors.New("input worker retired before readiness")

func readInputWorkerStartup(reader io.Reader) (*handoff.StartupProfile, Report, bool, error) {
	var prefix [8]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		_, err = receipt(Report{}, errors.Join(ErrProtocol, err))
		return nil, Report{}, false, err
	}
	reader = io.MultiReader(bytes.NewReader(prefix[:]), reader)
	if string(prefix[:4]) != "KRR1" {
		profile, err := readStartupReady(reader)
		return profile, Report{}, false, err
	}
	report, err := readReport(reader)
	report, err = receipt(report, err)
	if report.Source != nil || report.Completed != 0 || report.Recovery != (application.ReleaseReport{}) {
		err = errors.Join(err, ErrProtocol, ErrRecovery)
	}
	return nil, report, true, err
}

func readInputLeaseReady(reader io.Reader) (*handoff.StartupProfile, error) {
	profile, _, finished, err := readInputWorkerStartup(reader)
	if finished && !errors.Is(err, ErrRecovery) && !errors.Is(err, ErrProtocol) {
		return nil, errors.Join(ErrStartupRetired, err)
	}
	return profile, err
}

// This report grants no authority. The service calls it only after the exact
// child has reported empty recovery AND its owned job/process has been joined.
func publishInputStartupRetired(ctx context.Context, stream startupPublisher, report Report, result error) error {
	if ctx == nil || ctx.Err() != nil || report.Source != nil || report.Completed != 0 ||
		report.Recovery != (application.ReleaseReport{}) || errors.Is(result, ErrRecovery) || errors.Is(result, ErrProtocol) {
		return ErrAuthority
	}
	deadline := time.Now().Add(time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := stream.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return writeReport(stream, report, result)
}
