//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceInputPresenceCannotHoldHeartbeatGateOrLane(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
	defer w.channel.Close()
	w.started.Store(true)
	w.epoch.Store(5)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var checks atomic.Int32
	w.channel.authority.checkPresent = func() error {
		if checks.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil
	}
	input := make(chan error, 1)
	go func() {
		input <- w.SendAction(context.Background(), control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, nil)
	}()
	<-entered
	if w.gateOwner.Load() != 0 || !queryUsesProofCapacity(sourceWritePresenceAuthorityQuery) {
		t.Fatal("input presence owns serialization or the reserved heartbeat slot")
	}
	heartbeat := make(chan error, 1)
	go func() {
		heartbeat <- w.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 7}, nil)
	}()
	_ = server.SetReadDeadline(time.Now().Add(time.Second))
	first, err := readSourcePacket(server)
	if err != nil || first.op != sourceHeartbeat || first.sequence != 1 || <-heartbeat != nil {
		t.Fatal("input presence blocked actual heartbeat reply", first, err)
	}
	once.Do(func() { close(release) })
	second, err := readSourcePacket(server)
	if err != nil || <-input != nil || second.kind != sourceAction || second.sequence != 2 || checks.Load() != 2 {
		t.Fatal("input was lost, replayed or changed after heartbeat", second, err, checks.Load())
	}
}

func TestSourceReplyPresenceRechecksAfterContendedAdmission(t *testing.T) {
	synctest.Test(t, testSourceReplyPresenceRechecksAfterContendedAdmission)
}

func testSourceReplyPresenceRechecksAfterContendedAdmission(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
	defer w.channel.Close()
	var checks atomic.Int32
	checked := make(chan struct{})
	w.channel.authority.checkPresent = func() error {
		if checks.Add(1) == 1 {
			close(checked)
			return nil
		}
		return ErrAuthority
	}
	w.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- w.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 1}, nil) }()
	<-checked
	synctest.Wait()
	w.mu.Unlock()
	if err := <-done; !errors.Is(err, ErrAuthority) || checks.Load() != 2 || w.sequence != 0 {
		t.Fatal("reply reused presence proof across a gate wait", err, checks.Load(), w.sequence)
	}
}
