//go:build windows

package inputworker

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestPreparationOnlyEndpointRejectsEveryLeaseBeforeLoginAdmission(t *testing.T) {
	for _, lease := range []Lease{testLease(), func() Lease { l := testLease(); l.Capture = true; return l }(), func() Lease { l := testLease(); l.Presentation = true; return l }()} {
		peer, pipe := net.Pipe()
		channel := clientChannel(pipe, authorityAgent)
		data, err := encodeLease(lease, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		written := make(chan error, 1)
		go func() { written <- writeFrame(peer, data[:]) }()
		admissions := 0
		err = receivePreparationDemand(context.Background(), channel, func(context.Context) error { admissions++; return nil })
		if !errors.Is(err, ErrProtocol) || admissions != 0 {
			t.Fatal("preparation endpoint accepted a lease or reached admission", err)
		}
		if err := <-written; err != nil {
			t.Fatal(err)
		}
		_ = channel.Close()
		_ = peer.Close()
	}
}

func TestPreparationOnlyEndpointRequiresFreshProofAndLoginGate(t *testing.T) {
	for _, denied := range []bool{false, true} {
		peer, pipe := net.Pipe()
		channel := clientChannel(pipe, authorityAgent)
		var checks atomic.Int32
		channel.authority.checkInput = func() error { checks.Add(1); return nil }
		receipt := make(chan error, 1)
		go func() {
			data := encodePreparation()
			err := writeFrame(peer, data[:])
			if err == nil {
				var ack [8]byte
				_, err = io.ReadFull(peer, ack[:])
				if err == nil && ack != preparationReceipt {
					err = ErrProtocol
				}
			}
			receipt <- err
		}()
		admissions := 0
		err := receivePreparationDemand(context.Background(), channel, func(context.Context) error {
			admissions++
			if denied {
				return ErrAuthority
			}
			return nil
		})
		_ = channel.Close()
		ackErr := <-receipt
		_ = peer.Close()
		if admissions != 1 || denied && (!errors.Is(err, ErrAuthority) || ackErr == nil || checks.Load() != 2) || !denied && (err != nil || ackErr != nil || checks.Load() != 3) {
			t.Fatal("demand skipped fresh proofs or published a denied receipt", err, ackErr, admissions, checks.Load())
		}
	}
}

func TestConcurrentPreparationDemandTransfersOneOwnerAfterExactRetirement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var slot serviceInputPreparation
		ctx := context.Background()
		channel := &Channel{}
		if err := slot.admit(channel); err != nil {
			t.Fatal(err)
		}
		// Demand arrives while the main endpoint is doing login admission.
		release := make(chan struct{})
		child := &preparedConsole{preparedAt: time.Now()}
		var calls atomic.Int32
		build := func(context.Context) (*preparedConsole, error) { calls.Add(1); <-release; return child, nil }
		for range 8 {
			go slot.hint(ctx, time.Time{}, time.Now(), build)
		}
		synctest.Wait()
		transferred := make(chan *preparedConsole, 1)
		go func() {
			candidate, err := slot.take(channel, Lease{})
			if err != nil {
				t.Error(err)
			}
			transferred <- candidate
		}()
		synctest.Wait()
		select {
		case <-transferred:
			t.Fatal("entry bypassed pending retirement/construction")
		default:
		}
		if calls.Load() != 1 {
			t.Fatal("concurrent hints duplicated the spare", calls.Load())
		}
		close(release)
		if <-transferred != child {
			t.Fatal("lost exact prepared owner")
		}
		if next, err := slot.take(channel, Lease{}); next != nil || err != nil {
			t.Fatal("owner transferred twice", err)
		}
		_ = child.close()
		if err := slot.close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPreparationFailureRemainsTerminalAfterAnotherEndpointObservesIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var slot serviceInputPreparation
		cause := errors.New("retirement uncertain")
		slot.start(context.Background(), func(context.Context) (*preparedConsole, error) { return nil, errors.Join(ErrRecovery, cause) })
		synctest.Wait()
		if !errors.Is(slot.collectReady(), cause) {
			t.Fatal("hint endpoint lost failure")
		}
		channel := &Channel{}
		if candidate, err := slot.take(channel, Lease{}); candidate != nil || !errors.Is(err, cause) || !channel.closing {
			t.Fatal("input endpoint bypassed shared fence", err)
		}
		if slot.start(context.Background(), func(context.Context) (*preparedConsole, error) { t.Error("replacement passed fence"); return nil, nil }) {
			t.Fatal("terminal slot restarted")
		}
		if !errors.Is(slot.close(), cause) {
			t.Fatal("shutdown lost failure")
		}
	})
}
