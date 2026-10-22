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
				lifetime := &workerControlLifetime{stream: stream, proof: proof}
				service := lifetime.captureChannel()
				defer lifetime.Close()
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
				lifetime.interruptReader()
				if stream.closes.Load() != 0 || proofCloses.Load() != 0 {
					t.Fatal("reader interruption retired a live query")
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
				if err := lifetime.Close(); (queryFirst && err != nil) || (!queryFirst && !errors.Is(err, ErrAuthority)) {
					t.Fatal("control lifetime hid pending-query cleanup", err)
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

func TestWorkerControlLifetimeReleasesFailedBootstrap(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		peer, pipe := net.Pipe()
		stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
		lifetime := &workerControlLifetime{stream: stream}
		var proofCloses atomic.Int32
		if authenticated {
			lifetime.proof = clientChannel(stream, authorityService).authority
			lifetime.proof.closeHandles = func() error { proofCloses.Add(1); return nil }
		}
		if err := lifetime.Close(); err != nil {
			t.Fatal(err)
		}
		wantProofCloses := int32(0)
		if authenticated {
			wantProofCloses = 1
		}
		if stream.closes.Load() != 1 || proofCloses.Load() != wantProofCloses {
			t.Fatal("bootstrap failure leaked its pipe or authenticated proof")
		}
		_ = peer.Close()
	}
}
