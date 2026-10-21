package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestSourceFinalReportSurvivesExpiredServiceQuery(t *testing.T) {
	for _, queryFirst := range []bool{true, false} {
		name := "query_finishes_before_report"
		if !queryFirst {
			name = "report_finishes_before_query"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				peer, pipe := net.Pipe()
				defer peer.Close()
				stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
				proof := clientChannel(stream, authorityService).authority
				service, reporter := sourceReportChannel(stream, proof)
				defer reporter.Close()
				defer service.Close()
				entered, release := make(chan struct{}), make(chan struct{})
				var proofCloses atomic.Int32
				proof.closeHandles = func() error { proofCloses.Add(1); return nil }
				proof.checkInput = func() error {
					close(entered)
					<-release
					if proofCloses.Load() != 0 || stream.closes.Load() != 0 {
						t.Error("live native query lost its exact pipe or proof")
					}
					return nil
				}
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- sourceQueryOwner("service", service.authorizeSourceInput(ctx)) }()
				<-entered
				failure := <-done
				if !errors.Is(failure, context.DeadlineExceeded) || !proof.fenced.Load() {
					t.Fatal("expired proof was not fenced", failure)
				}
				if service.Close() != ErrAuthority || stream.closes.Load() != 0 {
					t.Fatal("pending query claimed confirmed cleanup")
				}
				if queryFirst {
					close(release)
					synctest.Wait()
				} else {
					failure = errors.Join(failure, ErrRecovery)
				}
				exit := cleanSourceExit()
				exit.Command, exit.Reader, exit.Retirement = "authority", "authority", "failed_sent"
				if !queryFirst {
					exit.Command, exit.Sender = "cleanup", "unjoined"
				}
				reported := make(chan error, 1)
				go func() { reported <- finishWorkerReport(stream, Report{Source: &exit}, failure) }()
				report, received := readReport(peer)
				returned := <-reported
				if report.Source == nil {
					t.Fatal("final report transport was retired before report delivery", received)
				}
				code, _ := reportFailureDetails(received)
				if reportOperations[code] != "source-service-query-deadline" || !errors.Is(returned, failure) || errors.Is(received, ErrRecovery) == queryFirst {
					t.Fatal("query failure or uncertain cleanup was hidden", received, returned)
				}
				if service.authorizeSourcePresence(context.Background()) == nil {
					t.Fatal("report ownership revived input authority")
				}
				if err := reporter.Close(); err != nil {
					t.Fatal(err)
				}
				if queryFirst {
					if stream.closes.Load() != 1 || proofCloses.Load() != 1 {
						t.Fatal("completed report/query ownership leaked resources")
					}
				} else {
					if stream.closes.Load() != 0 || proofCloses.Load() != 0 {
						t.Fatal("report completion retired a still-running native query")
					}
					close(release)
					synctest.Wait()
					if stream.closes.Load() != 1 || proofCloses.Load() != 1 {
						t.Fatal("late query did not retire the exact pipe/proof once")
					}
				}
			})
		})
	}
}
