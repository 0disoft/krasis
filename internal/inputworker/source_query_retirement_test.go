package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestCompletedReadAuthorizationDeadlineDoesNotRequireServiceRepair(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityService)
		defer owner.Close()
		release := make(chan struct{})
		owner.authority.checkPresent = func() error { <-release; return nil }
		request := sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 1, binding: Binding{1}}
		encoded, err := encodeSourcePacket(request)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, failure := readSourceCommandAuthorized(bytes.NewReader(encoded[:]), context.Background(), request.binding, 0, func(byte) error {
			return sourceQueryOwner("service", owner.authorizeSourcePresence(ctx))
		})
		if !errors.Is(failure, context.DeadlineExceeded) || !owner.authority.fenced.Load() {
			t.Fatal("query deadline no longer rejects the command", failure)
		}
		close(release)
		synctest.Wait()
		if owner.Close() != nil {
			t.Fatal("query did not finish its owned cleanup")
		}
		final := sourceCaptureFinal(failure, inputdesktop.ErrInactive, context.Canceled, false)
		final = sourceCaptureQueryRetirement(final, owner)
		var report bytes.Buffer
		if err := writeReport(&report, Report{}, final); err != nil {
			t.Fatal(err)
		}
		_, received := readReport(&report)
		code, _ := reportFailureDetails(received)
		if final == nil || errors.Is(final, ErrRecovery) || sourceRetirementStatus(final) != 0 || serviceRecoveryFailure(received, nil) != nil || reportOperations[code] != "source-service-query-deadline-checking" {
			t.Fatal("completed read-only query became uncertain native cleanup", final, reportOperations[code])
		}
	})
}

func TestReadQueryRejectionCannotHideIndependentCleanupFailure(t *testing.T) {
	query := sourceQueryOwner("service", queryPhaseFailure(sourcePresenceAuthorityQuery, "checking", context.DeadlineExceeded))
	rejected := &sourceReaderFailure{operation: "source-reader-authority", cause: query}
	for _, result := range []error{
		sourceCaptureFinal(rejected, inputdesktop.ErrCleanup, context.Canceled, false),
		sourceCaptureFinal(rejected, context.Canceled, context.DeadlineExceeded, false),
		sourceCaptureFinal(errors.Join(rejected, errors.New("unknown failure")), context.Canceled, context.Canceled, false),
		sourceCaptureFinal(query, context.Canceled, context.Canceled, false),
		sourceCaptureFinal(&sourceReaderFailure{operation: "source-reader-invalid", cause: query}, context.Canceled, context.Canceled, false),
		sourceCaptureFinal(&sourceReaderFailure{operation: "source-reader-authority", cause: &sourceQueryFailure{reason: "private", cause: errors.Join(ErrAuthority, context.DeadlineExceeded)}}, context.Canceled, context.Canceled, false),
		sourceCaptureFinal(&sourceReaderFailure{operation: "source-reader-authority", cause: &sourceQueryFailure{reason: "deadline", cause: errors.Join(ErrAuthority, context.DeadlineExceeded, errors.New("unknown query failure"))}}, context.Canceled, context.Canceled, false),
		sourceCaptureQueryRetirement(sourceCaptureFinal(rejected, context.Canceled, context.Canceled, false), nil),
	} {
		if !errors.Is(result, ErrRecovery) || sourceRetirementStatus(result) != 1 {
			t.Fatal("query rejection hid independent uncertainty", result)
		}
	}
}
