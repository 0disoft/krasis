package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestSourceQueryWaitingDeadlineDoesNotOwnPreviousQuery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityAgent)
		defer owner.Close()
		entered, release := make(chan struct{}), make(chan struct{})
		var checks atomic.Int32
		owner.authority.checkPresent = func() error { checks.Add(1); close(entered); <-release; return nil }
		done := make(chan error, 1)
		go func() { done <- owner.authorizeSourcePresence(context.Background()) }()
		<-entered
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		failure := owner.authorizeSourcePresence(ctx)
		var query *sourceQueryFailure
		if !errors.Is(failure, context.DeadlineExceeded) || !errors.As(failure, &query) || query.reason != "deadline-waiting" || owner.authority.fenced.Load() || checks.Load() != 1 {
			t.Fatal("waiter lost phase or changed previous owner", failure, query, checks.Load())
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal("waiter retired another caller's query", err)
		}
		synctest.Wait()
		if owner.Close() != nil {
			t.Fatal("completed query leaked handles")
		}
	})
}

func TestSourceQueryPhaseSurvivesProjectionAndReport(t *testing.T) {
	for _, phase := range []string{"admission", "waiting", "scheduled", "checking", "publishing", "retiring", "completed"} {
		failure := sourceQueryOwner("service", queryPhaseFailure(sourcePresenceAuthorityQuery, phase, context.DeadlineExceeded))
		if !errors.Is(failure, context.DeadlineExceeded) {
			t.Fatal("phase lost caller deadline")
		}
		projected := sourceAuthorityResult(failure)
		final := sourceCaptureFinal(&sourceReaderFailure{operation: "source-reader-authority", cause: projected}, inputdesktop.ErrInactive, context.Canceled, false)
		var wire bytes.Buffer
		if err := writeReport(&wire, Report{}, final); err != nil {
			t.Fatal(err)
		}
		_, received := readReport(&wire)
		code, _ := reportFailureDetails(received)
		if reportOperations[code] != "source-service-query-deadline-"+phase || errors.Is(final, ErrRecovery) {
			t.Fatal("projection changed phase or cleanup classification", phase, final)
		}
	}
	for _, operation := range logindiag.SourceQueryDeadlineOperations() {
		if !logindiag.ValidRuntimeOperation(operation) {
			t.Fatal("fixed phase cannot be exported", operation)
		}
	}
	for _, operation := range []string{"source-service-query-deadline-private", "source-private-query-deadline-checking", "source-query-deadline-checking-private"} {
		if logindiag.ValidRuntimeOperation(operation) {
			t.Fatal("private label admitted", operation)
		}
	}
	if len(reportOperations) > 256 {
		t.Fatal("operation code byte overflow")
	}
	ordinary := queryPhaseFailure(inputAuthorityQuery, "checking", context.DeadlineExceeded)
	var source *sourceQueryFailure
	if !errors.Is(ordinary, ErrAuthority) || !errors.Is(ordinary, context.DeadlineExceeded) || errors.As(ordinary, &source) {
		t.Fatal("ordinary caller contract changed")
	}
}
