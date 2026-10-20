package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestJournalRecoveryChildRetiresEvenWhenNativeReleaseIgnoresCancellation(t *testing.T) {
	for _, mode := range []string{"release", "release-hang"} {
		t.Run(mode, func(t *testing.T) {
			process, client := startPortableFixture(t, "normal")
			releaseProcess, releaseClient := startPortableFixture(t, mode)
			input := newIsolatedInput(client, process, releaseClient)
			releaseOwner := newIsolatedInput(releaseClient, releaseProcess, nil)
			input.recoveryRetire = releaseOwner.retire
			server, remote := net.Pipe()
			lease := testLease()
			done := make(chan finish, 1)
			channel := clientChannel(server, authorityAgent)
			parentChecks := 0
			forbiddenParent := func() error { parentChecks++; return errors.New("parent native recovery query ran") }
			channel.authority.checkInput, channel.authority.checkRecovery = forbiddenParent, forbiddenParent
			go func() {
				report, err := serveIsolatedJournal(context.Background(), channel, input, lease, nil)
				done <- finish{report, err}
			}()
			send(t, remote, request{op: opPress, sequence: 1, binding: lease.Binding, token: loginTestKey})
			started := time.Now()
			_ = remote.Close()
			select {
			case got := <-done:
				if mode == "release" {
					if got.err != nil || got.report.Recovery.Released != 1 || got.report.Recovery.Remaining != 0 {
						t.Fatal(got)
					}
				} else if !errors.Is(got.err, ErrRecovery) || got.report.Recovery.Remaining != 1 || got.report.Recovery.Released != 0 {
					t.Fatal("unacknowledged release lost held state", got)
				}
			case <-time.After(8 * time.Second):
				t.Fatal("native recovery blocked journal owner")
			}
			for _, child := range []<-chan struct{}{process.Done(), releaseProcess.Done()} {
				select {
				case <-child:
				default:
					t.Fatal("recovery returned before exact child exit")
				}
			}
			if time.Since(started) > 8*time.Second {
				t.Fatal("recovery exceeded bound")
			}
			if parentChecks != 0 {
				t.Fatal("journal owner queried native authority", parentChecks)
			}
		})
	}
}

func TestEmptyJournalCannotHideUnretiredRecoveryChild(t *testing.T) {
	count := 0
	input := &isolatedInput{Client: &Client{}, retire: func() (bool, error) { return true, nil }, recoveryRetire: func() (bool, error) { count++; return false, ErrAuthority }}
	server, remote := net.Pipe()
	_ = remote.Close()
	report, err := serve(context.Background(), server, input, testLease(), testExecute, testExecute, time.Now)
	if !errors.Is(err, ErrRecovery) || report.Recovery.Remaining != 0 || count != 1 {
		t.Fatal(report, err, count)
	}
	if !errors.Is(input.finishRecovery(), ErrRecovery) || count != 1 {
		t.Fatal("uncertain retirement was retried", count)
	}
}
