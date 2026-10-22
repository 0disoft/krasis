//go:build windows

package inputworker

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceWriterSlowAuthorizationYieldsToHeartbeat(t *testing.T) {
	for _, kind := range []byte{sourceAction, sourceReturn, sourceReply} {
		t.Run(map[byte]string{sourceAction: "input", sourceReturn: "return", sourceReply: "command-ack"}[kind], func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			w.started.Store(true)
			entered, release := make(chan struct{}), make(chan struct{})
			var announced atomic.Bool
			w.channel.authority.checkInput = func() error {
				if !announced.Swap(true) {
					close(entered)
				}
				<-release
				return nil
			}
			w.channel.authority.checkPresent = func() error { return nil }
			data := sourcePacket{kind: kind, reply: 1}
			if kind == sourceAction {
				data.action = control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
			}
			if kind == sourceReply {
				data.op = sourceActivate
			}
			dataDone, heartbeatDone := make(chan error, 1), make(chan error, 1)
			go func() { dataDone <- w.send(data, nil) }()
			<-entered
			go func() { heartbeatDone <- w.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 2}, nil) }()
			_ = server.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
			first, firstErr := readSourcePacket(server)
			close(release)
			_ = server.SetReadDeadline(time.Now().Add(time.Second))
			second, secondErr := readSourcePacket(server)
			if firstErr != nil {
				_, _ = readSourcePacket(server)
			}
			dataErr, heartbeatErr := <-dataDone, <-heartbeatDone
			if firstErr != nil || secondErr != nil || dataErr != nil || heartbeatErr != nil || first.op != sourceHeartbeat || first.sequence != 1 || second.kind != kind || second.sequence != 2 {
				t.Fatal("slow native authorization held the heartbeat writer", first, second, firstErr, secondErr, dataErr, heartbeatErr)
			}
		})
	}
}

func TestSourceWriterReauthorizesAfterContendedAdmission(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
	defer w.channel.Close()
	w.started.Store(true)
	var checks atomic.Int32
	checked := make(chan struct{})
	w.channel.authority.checkInput = func() error {
		if checks.Add(1) == 1 {
			close(checked)
			return nil
		}
		return ErrAuthority
	}
	w.channel.authority.checkPresent = func() error { return nil }
	w.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- w.send(sourcePacket{kind: sourceReturn, reply: 1}, nil) }()
	<-checked
	time.Sleep(20 * time.Millisecond) // sync.Mutex waits do not advance synctest time.
	w.mu.Unlock()
	if err := <-done; err == nil || checks.Load() != 2 || w.sequence != 0 {
		t.Fatal("stale full authorization survived writer contention", err, checks.Load(), w.sequence)
	}
}

func TestSourceWriterRechecksOwnerAndGenerationBeforeWrite(t *testing.T) {
	for _, changed := range []string{"owner", "generation"} {
		t.Run(changed, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			w := &sourceWriter{channel: clientChannel(conn, authorityAgent), binding: Binding{1}}
			defer w.channel.Close()
			w.started.Store(true)
			entered, release := make(chan struct{}), make(chan struct{})
			w.channel.authority.checkInput = func() error { close(entered); <-release; return nil }
			var current atomic.Bool
			current.Store(true)
			w.channel.authority.checkPresent = func() error {
				if changed == "owner" && !current.Load() {
					return ErrAuthority
				}
				return nil
			}
			validGeneration := func() bool { return changed != "generation" || current.Load() }
			done := make(chan error, 1)
			go func() {
				done <- w.send(sourcePacket{kind: sourceAction, reply: 1, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}}, validGeneration)
			}()
			<-entered
			current.Store(false)
			close(release)
			err := <-done
			if err == nil || w.sequence != 0 || changed == "generation" && !errors.Is(err, control.ErrSourceActionDiscarded) {
				t.Fatal("obsolete source authority or generation reached pipe", err, w.sequence)
			}
		})
	}
}
