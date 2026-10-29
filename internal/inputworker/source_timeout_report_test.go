package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestSourceReadTimeoutReportDistinguishesRetirementFailure(t *testing.T) {
	for _, kind := range []authorityQueryKind{sourceInputAuthorityQuery, sourcePresenceAuthorityQuery, sourceWatchdogAuthorityQuery, sourceAdvisoryPresenceAuthorityQuery} {
		for _, state := range []string{"joined", "late-joined", "pending", "close-failed"} {
			t.Run(state+"/lane-"+strconv.Itoa(int(kind)), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					defer isolateAuthorityCapacity(t)()
					peer, pipe := net.Pipe()
					defer peer.Close()
					owner := clientChannel(pipe, authorityService)
					release := make(chan struct{})
					defer func() { close(release); synctest.Wait(); _ = owner.Close() }()
					check := func() error {
						if state != "joined" {
							<-release
						}
						return nil
					}
					owner.authority.checkInput, owner.authority.checkPresent = check, check
					if state == "close-failed" {
						owner.authority.closeHandles = func() error { return syscall.Errno(5) }
					}
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
					defer cancel()
					queryErr := owner.queryAuthority(ctx, false, kind)
					if (state == "joined" && queryErr != nil) || (state != "joined" && !errors.Is(queryErr, context.DeadlineExceeded)) {
						t.Fatal("query did not reach the required retirement state", queryErr)
					}
					if state == "late-joined" || state == "close-failed" {
						// A pending Close fences optional queries too, without releasing
						// native handles still owned by any of the four lanes.
						_ = owner.Close()
						time.AfterFunc(50*time.Millisecond, func() { release <- struct{}{} })
					}
					failure := sourceCaptureFinal(sourceReadFailure(os.ErrDeadlineExceeded), inputdesktop.ErrInactive, context.Canceled, false)
					started := time.Now()
					final := sourceCaptureQueryRetirement(failure, owner)
					wait := time.Since(started)
					wantWait := time.Duration(0)
					if state == "pending" {
						wantWait = RecoveryTimeout
					}
					if state == "late-joined" || state == "close-failed" {
						wantWait = 50 * time.Millisecond
					}
					if wait != wantWait {
						t.Fatalf("retirement wait=%v want=%v", wait, wantWait)
					}
					var wire bytes.Buffer
					if err := writeReport(&wire, Report{}, final); err != nil {
						t.Fatal(err)
					}
					_, received := readReport(&wire)
					code, _ := reportFailureDetails(received)
					uncertain := state == "pending" || state == "close-failed"
					want := "source-reader-timeout"
					if uncertain {
						want = "source-channel-close"
					}
					if reportOperations[code] != want || errors.Is(received, ErrRecovery) != uncertain || (serviceRecoveryFailure(received, nil) != nil) != uncertain || !errors.Is(final, os.ErrDeadlineExceeded) {
						t.Fatalf("state=%s operation=%s recovery=%t final=%v", state, reportOperations[code], errors.Is(received, ErrRecovery), final)
					}
				})
			})
		}
	}
}

func TestSourceReadRecoveryReportsNativeCodeFromCleanupBranch(t *testing.T) {
	for _, operation := range []string{"source-reader-read", "source-reader-eof", "source-reader-truncated", "source-reader-timeout", "source-reader-closed"} {
		read := &workerReportFailure{operation: operation, cause: syscall.Errno(109)}
		cleanup := &workerReportFailure{operation: "source-channel-close", cause: syscall.Errno(5)}
		for _, recovery := range []bool{false, true} {
			result := errors.Join(read, cleanup)
			want, native := operation, uint32(109)
			if recovery {
				result = errors.Join(result, ErrRecovery)
				want, native = "source-channel-close", 5
			}
			var wire bytes.Buffer
			if err := writeReport(&wire, Report{}, result); err != nil {
				t.Fatal(err)
			}
			_, received := readReport(&wire)
			code, number := reportFailureDetails(received)
			if reportOperations[code] != want || number != native || errors.Is(received, ErrRecovery) != recovery {
				t.Fatalf("operation=%s native=%d recovery=%t", reportOperations[code], number, errors.Is(received, ErrRecovery))
			}
		}
	}
}
