//go:build windows

package inputworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestNativeActionBackpressureReservesHeartbeatAndRetirementSpace(t *testing.T) {
	server, agent := nativePair(t)
	run, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &sourceWriter{ctx: run, channel: clientChannel(server, authorityAgent), binding: Binding{1}, boundary: newSourceBoundaryWindow(), actions: newSourceActionWindow()}
	defer w.channel.Close()
	w.started.Store(true)
	batch := make([]sourcePacket, sourceBoundaryBatchLimit)
	for i := range batch {
		batch[i] = boundaryBatchPacket(int32(i))
	}
	if err := w.sendBoundaries(batch); err != nil {
		t.Fatal(err)
	}
	w.epoch.Store(42)
	for i := 1; i <= sourceActionWindowLimit; i++ {
		if err := w.SendAction(run, control.InputAction{Kind: control.InputPointerMove, DeltaX: int32(i)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	// The app stops reading longer than the former one-second write budget.
	wait, stop := context.WithCancel(run)
	done := make(chan error, 1)
	go func() { done <- w.SendAction(wait, control.InputAction{Kind: control.InputKeyDown, Code: 1}, nil) }()
	defer func() { stop(); <-done }()
	time.Sleep(1100 * time.Millisecond)
	if w.gateOwner.Load() != 0 {
		t.Fatal("input credit waiter owns wire gate")
	}
	select {
	case err := <-done:
		done <- err
		t.Fatal("credit wait consumed write budget", err)
	default:
	}
	if err := w.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 7}, nil); err != nil {
		t.Fatal("input filled heartbeat space", err)
	}
	if err := w.write(sourcePacket{kind: sourceRetired}, nil, true); err != nil {
		t.Fatal("input filled retirement space", err)
	}
	_ = agent.SetReadDeadline(time.Now().Add(time.Second))
	for i := 1; i <= sourceBoundaryBatchLimit+sourceActionWindowLimit+2; i++ {
		p, err := readSourcePacket(agent)
		if err != nil || p.sequence != uint64(i) {
			t.Fatal("wire order changed", p, err)
		}
		if i > sourceBoundaryBatchLimit && i <= sourceBoundaryBatchLimit+sourceActionWindowLimit && p.action.DeltaX != int32(i-sourceBoundaryBatchLimit) {
			t.Fatal("input reordered", p)
		}
	}
	if w.broken {
		t.Fatal("backpressure poisoned stream")
	}
}

func TestSourceActionReceiptUnblocksOnlyExactGroup(t *testing.T) {
	server, agent := nativePair(t)
	run, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &sourceWriter{ctx: run, channel: clientChannel(server, authorityAgent), binding: Binding{1}, boundary: newSourceBoundaryWindow(), actions: newSourceActionWindow()}
	defer w.channel.Close()
	w.started.Store(true)
	w.epoch.Store(42)
	for i := 1; i <= sourceActionWindowLimit; i++ {
		if err := w.SendAction(run, control.InputAction{Kind: control.InputPointerMove, DeltaX: int32(i)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	receipt := sourcePacket{kind: sourceCommand, op: sourceBoundaryAck, binding: w.binding, sequence: 1, reply: 7}
	if w.handleSourceMaintenance(receipt) != ErrProtocol {
		t.Fatal("bad action reference accepted")
	}
	// Stop waiting input using the actual caller context: no credit/sequence
	// may escape, then the exact receipt permits a distinct fresh action.
	wait, stop := context.WithTimeout(run, 25*time.Millisecond)
	defer stop()
	if err := w.SendAction(wait, control.InputAction{Kind: control.InputKeyDown, Code: 1}, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if w.sequence != 16 || w.broken {
		t.Fatal("unsent waiter changed stream")
	}
	receipt.reply = 8
	if err := w.handleSourceMaintenance(receipt); err != nil {
		t.Fatal(err)
	}
	if w.handleSourceMaintenance(receipt) != ErrProtocol {
		t.Fatal("duplicate receipt accepted")
	}
	if err := w.SendAction(run, control.InputAction{Kind: control.InputKeyDown, Code: 1}, func() bool { return false }); !errors.Is(err, control.ErrSourceActionDiscarded) || len(w.actions.credit) != 8 {
		t.Fatal("stale generation retained credit", err)
	}
	if err := w.SendAction(run, control.InputAction{Kind: control.InputKeyDown, Code: 1}, nil); err != nil {
		t.Fatal(err)
	}
	_ = agent.SetReadDeadline(time.Now().Add(time.Second))
	for i := 1; i <= 17; i++ {
		p, err := readSourcePacket(agent)
		if err != nil || p.sequence != uint64(i) {
			t.Fatal(p, err)
		}
	}
	if w.sequence != 17 || w.broken {
		t.Fatal("credit altered sequence")
	}
}

func TestNativeActionCreditRoundTripPreservesOrderAcrossEpochs(t *testing.T) {
	server, agent := nativePair(t)
	run, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w := &sourceWriter{ctx: run, channel: clientChannel(server, authorityAgent), binding: Binding{1}, boundary: newSourceBoundaryWindow(), actions: newSourceActionWindow()}
	defer w.channel.Close()
	w.started.Store(true)
	c := &sourceClient{channel: clientChannel(agent, authorityService), binding: w.binding, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	const total = 128
	readDone, receiptDone := make(chan error, 1), make(chan error, 1)
	go func() {
		for sequence := uint64(1); sequence <= total/sourceActionReceiptEvery; sequence++ {
			p, err := readSourceCommand(server, run, w.binding, sequence-1, func() error { return nil })
			if err == nil {
				err = w.handleSourceMaintenance(p)
			}
			if err != nil {
				receiptDone <- err
				return
			}
		}
		receiptDone <- nil
	}()
	go func() {
		for i := 1; i <= total; i++ {
			p, err := readSourcePacket(agent)
			if err != nil {
				readDone <- err
				return
			}
			if p.sequence != uint64(i) || p.action.DeltaX != int32(i) || p.reply != uint64((i-1)/13+1) {
				readDone <- ErrProtocol
				return
			}
			if i%sourceActionReceiptEvery == 0 {
				if _, err := c.call(run, sourceBoundaryAck, p.sequence); err != nil {
					readDone <- err
					return
				}
			}
		}
		readDone <- nil
	}()
	for i := 1; i <= total; i++ {
		w.epoch.Store(uint64((i-1)/13 + 1)) // Groups must span lease changes.
		if err := w.SendAction(run, control.InputAction{Kind: control.InputPointerMove, DeltaX: int32(i)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := <-receiptDone; err != nil {
		t.Fatal(err)
	}
	if len(w.actions.credit) != sourceActionWindowLimit || w.sequence != total || w.broken || !c.lastReplyRequestAt.IsZero() {
		t.Fatal("receipts leaked capacity or renewed liveness")
	}
}
