package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestSourceQueryRetirementJoinsLateNativeOwnerBeforeServiceReport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityService)
		defer owner.Close()
		release := make(chan struct{})
		var closed atomic.Int32
		owner.authority.closeHandles = func() error { closed.Add(1); return nil }
		owner.authority.checkPresent = func() error { <-release; return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		failure := sourceQueryOwner("service", owner.authorizeSourcePresence(ctx))
		owner.queryMu.Lock()
		query := owner.presenceQuery
		owner.queryMu.Unlock()
		if query == nil || !owner.authority.fenced.Load() || closed.Load() != 0 {
			t.Fatal("expired native owner was not retained and fenced")
		}
		time.AfterFunc(50*time.Millisecond, func() { close(release) })
		started := time.Now()
		final := sourceCaptureFinal(&sourceReaderFailure{operation: "source-reader-authority", cause: failure}, inputdesktop.ErrInactive, context.Canceled, false)
		final = sourceCaptureQueryRetirement(final, owner)
		elapsed := time.Since(started)
		<-query.done
		var wire bytes.Buffer
		if err := writeReport(&wire, Report{}, final); err != nil {
			t.Fatal(err)
		}
		_, received := readReport(&wire)
		if elapsed != 50*time.Millisecond || final == nil || errors.Is(final, ErrRecovery) || sourceRetirementStatus(final) != 0 || serviceRecoveryFailure(received, nil) != nil {
			t.Fatal("late confirmed query retirement unnecessarily stopped service", elapsed, final)
		}
		if closed.Load() != 1 || owner.authorizeSourcePresence(context.Background()) == nil {
			t.Fatal("joining cleanup lost handles or revived expired authority")
		}
	})
}

func TestSourceQueryRetirementSharesOneBudgetAndRetainsUnjoinedOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		var owners []*Channel
		var releases []chan struct{}
		var once [2]sync.Once
		var closes [2]atomic.Int32
		for i := range 2 {
			server, pipe := net.Pipe()
			defer server.Close()
			owner := clientChannel(pipe, authorityService)
			defer owner.Close()
			release := make(chan struct{})
			owner.authority.checkPresent = func() error { <-release; return nil }
			owner.authority.closeHandles = func() error { closes[i].Add(1); return nil }
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			if err := owner.authorizeSourcePresence(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("query did not expire with retained native work", err)
			}
			cancel()
			owners, releases = append(owners, owner), append(releases, release)
		}
		defer func() {
			for i := range releases {
				once[i].Do(func() { close(releases[i]) })
			}
			synctest.Wait()
		}()
		time.AfterFunc(RecoveryTimeout*3/4, func() { once[0].Do(func() { close(releases[0]) }) })
		started := time.Now()
		final := sourceCaptureQueryRetirement(ErrAuthority, owners...)
		elapsed := time.Since(started)
		if elapsed != RecoveryTimeout || !errors.Is(final, ErrRecovery) || sourceRetirementStatus(final) != 1 || closes[0].Load() != 1 || closes[1].Load() != 0 {
			t.Fatal("retirement renewed its budget or certified an unjoined owner", elapsed, final)
		}
		if owners[1].Close() != ErrAuthority || closes[1].Load() != 0 {
			t.Error("pending query lost its exact native resources")
		}
		once[1].Do(func() { close(releases[1]) })
		synctest.Wait()
		if closes[1].Load() != 1 {
			t.Fatal("late native return did not retire exact resources")
		}
	})
}

func TestSourceQueryRetirementPreservesLateHandleCloseFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityService)
		defer owner.Close()
		release := make(chan struct{})
		owner.authority.checkPresent = func() error { <-release; return nil }
		owner.authority.closeHandles = func() error { return ErrAuthority }
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		failure := sourceQueryOwner("service", owner.authorizeSourcePresence(ctx))
		time.AfterFunc(50*time.Millisecond, func() { close(release) })
		final := sourceCaptureFinal(&sourceReaderFailure{operation: "source-reader-authority", cause: failure}, inputdesktop.ErrInactive, context.Canceled, false)
		final = sourceCaptureQueryRetirement(final, owner)
		var wire bytes.Buffer
		if err := writeReport(&wire, Report{}, final); err != nil {
			t.Fatal(err)
		}
		_, received := readReport(&wire)
		if !errors.Is(final, ErrRecovery) || sourceRetirementStatus(final) != 1 || serviceRecoveryFailure(received, nil) == nil {
			t.Fatal("completed query hid failed native handle retirement", final)
		}
	})
}
