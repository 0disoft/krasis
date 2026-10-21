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

func TestSourceReadQueryDeadlineIdentifiesOwnerThroughFinalReport(t *testing.T) {
	for _, stalled := range []int{0, 1} {
		synctest.Test(t, func(t *testing.T) {
			owners := make([]*Channel, 2)
			for i := range owners {
				server, pipe := net.Pipe()
				defer server.Close()
				owners[i] = clientChannel(pipe, authorityService)
				defer owners[i].Close()
			}
			release := make(chan struct{})
			owners[stalled].authority.checkPresent = func() error { <-release; return nil }
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			failure := authorizeSourceRead(ctx, owners[0], owners[1], sourceHeartbeat)
			if !errors.Is(failure, ErrAuthority) || !errors.Is(failure, context.DeadlineExceeded) {
				t.Fatal("query outcome changed", failure)
			}
			close(release)
			synctest.Wait()
			// The watcher sanitization, final classification and wire used to
			// collapse this into a generic source-reader-authority operation.
			observed := sourceAuthorityResult(failure)
			final := sourceCaptureFinal(&sourceReaderFailure{operation: "source-reader-authority", cause: observed}, inputdesktop.ErrInactive, context.Canceled, false)
			var wire bytes.Buffer
			exit := cleanSourceExit()
			if err := writeReport(&wire, Report{Source: &exit}, final); err != nil {
				t.Fatal(err)
			}
			_, received := readReport(&wire)
			code, native := reportFailureDetails(received)
			want := []string{"source-service-query-deadline", "source-agent-query-deadline"}[stalled]
			if reportOperations[code] != want || native != 0 || errors.Is(final, ErrRecovery) {
				t.Fatal("query provenance or retirement classification lost", reportOperations[code], native, final)
			}
		})
	}
}

func TestSourceQueryCapacityAndClosedAreDifferentFromNativeDenial(t *testing.T) {
	server, pipe := net.Pipe()
	defer server.Close()
	channel := clientChannel(pipe, authorityService)
	defer channel.Close()
	if len(authorityQueries) != 0 {
		t.Fatal("unfinished query from earlier test")
	}
	for i := 0; i < cap(authorityQueries); i++ {
		authorityQueries <- struct{}{}
	}
	failure := channel.authorizeSourcePresence(context.Background())
	for i := 0; i < cap(authorityQueries); i++ {
		<-authorityQueries
	}
	code, _ := reportFailureDetails(failure)
	if reportOperations[code] != "source-query-capacity" || !errors.Is(failure, ErrAuthority) {
		t.Fatal("capacity failure lost", failure)
	}
	channel.authority.fenced.Store(true)
	code, _ = reportFailureDetails(channel.authorizeSourcePresence(context.Background()))
	if reportOperations[code] != "source-query-closed" {
		t.Fatal("closed query lost", reportOperations[code])
	}
}

func TestSourceQueryFailureDoesNotReplaceConcreteNativeFailure(t *testing.T) {
	query := queryAdmissionFailure(sourcePresenceAuthorityQuery, "", context.DeadlineExceeded)
	native := authorityCheckFailed("authority-process-exited", nil)
	code, _ := reportFailureDetails(errors.Join(query, native))
	if reportOperations[code] != "authority-process-exited" {
		t.Fatal("query erased native rejection", reportOperations[code])
	}
	for _, kind := range []authorityQueryKind{inputAuthorityQuery, presenceAuthorityQuery, heartbeatAuthorityQuery} {
		if queryAdmissionFailure(kind, "capacity", nil) != ErrAuthority {
			t.Fatal("ordinary query sentinel changed")
		}
	}
}
