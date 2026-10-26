//go:build windows

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

func TestAgentRequestAuthorityUsesWholeHandshakeBudget(t *testing.T) {
	for _, blockedCheck := range []int32{1, 2} {
		for _, budget := range []time.Duration{50 * time.Millisecond, startupTimeout} {
			t.Run(map[int32]string{1: "before_request", 2: "after_request"}[blockedCheck]+"/"+budget.String(), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					defer isolateAuthorityCapacity(t)()
					peer, pipe := net.Pipe()
					defer peer.Close()
					stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
					owner := clientChannel(stream, authorityAgent)
					defer owner.Close()
					var checks, closes atomic.Int32
					owner.authority.closeHandles = func() error { closes.Add(1); return nil }
					owner.authority.checkInput = func() error {
						if checks.Add(1) == blockedCheck {
							time.Sleep(2 * budget)
							if stream.closes.Load() != 0 || closes.Load() != 0 {
								t.Error("startup cancellation closed handles still owned by native authorization")
							}
						}
						return nil
					}
					var written chan error
					if blockedCheck == 2 {
						written = make(chan error, 1)
						go func() { packet := encodePreparation(); _, err := peer.Write(packet[:]); written <- err }()
					}
					ctx := context.Background()
					if budget < startupTimeout {
						var cancel context.CancelFunc
						ctx, cancel = context.WithTimeout(ctx, budget)
						defer cancel()
					}
					started := time.Now()
					_, prepare, err := receiveAgentRequest(ctx, owner)
					if !errors.Is(err, ErrAuthority) || prepare || time.Since(started) != budget {
						t.Errorf("native authority escaped handshake budget: elapsed=%s prepare=%t err=%v", time.Since(started), prepare, err)
					}
					if !owner.authority.fenced.Load() || owner.Close() != ErrAuthority || stream.closes.Load() != 0 || closes.Load() != 0 {
						t.Error("expired native authorization was not fenced and retained")
					}
					time.Sleep(2 * budget)
					synctest.Wait()
					if owner.Close() != nil || stream.closes.Load() != 1 || closes.Load() != 1 {
						t.Error("late authorization did not retire its resources exactly once")
					}
					if written != nil {
						if err := <-written; err != nil {
							t.Error(err)
						}
					}
				})
			})
		}
	}
}

func TestAgentRequestAuthorityCannotBypassNativeCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		_, finish := occupyAuthorityQueries(t)
		defer finish()
		peer, pipe := net.Pipe()
		defer peer.Close()
		owner := clientChannel(pipe, authorityAgent)
		defer owner.Close()
		var checks atomic.Int32
		owner.authority.checkInput = func() error { checks.Add(1); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, prepare, err := receiveAgentRequest(ctx, owner)
		if !errors.Is(err, ErrAuthority) || prepare || checks.Load() != 0 || len(authorityProofQueries) != 0 {
			t.Fatal("startup bypassed the native query limit or leaked a reservation", err, checks.Load())
		}
	})
}

func TestAgentRequestFreshChecksPreserveLeaseAndPreparation(t *testing.T) {
	for _, preparation := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			peer, pipe := net.Pipe()
			defer peer.Close()
			stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
			owner := clientChannel(stream, authorityAgent)
			defer owner.Close()
			var checks atomic.Int32
			owner.authority.checkInput = func() error { checks.Add(1); return nil }
			want := testLease()
			packet, err := encodeLease(want, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if preparation {
				packet = encodePreparation()
			}
			written := make(chan error, 1)
			go func() { _, err := peer.Write(packet[:]); written <- err }()
			got, prepared, err := receiveAgentRequest(context.Background(), owner)
			if err != nil || prepared != preparation || checks.Load() != 2 {
				t.Fatal("startup did not retain two fresh full checks", prepared, checks.Load(), err)
			}
			if !preparation && (got.Identity != want.Identity || got.Binding != want.Binding || !got.ExpiresAt.Equal(want.ExpiresAt)) {
				t.Fatal("startup changed authenticated lease metadata")
			}
			if err := <-written; err != nil {
				t.Fatal(err)
			}
			time.Sleep(startupTimeout + time.Millisecond)
			synctest.Wait()
			if owner.authority.fenced.Load() || stream.closes.Load() != 0 {
				t.Fatal("finished startup cancellation callback closed the accepted channel")
			}
		})
	}
}
