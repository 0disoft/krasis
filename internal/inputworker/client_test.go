package inputworker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

func clientChannel(stream net.Conn, kind authorityKind) *Channel {
	return &Channel{stream: stream, authority: &Authority{kind: kind, checkInput: func() error { return nil }, checkRecovery: func() error { return nil }, closeHandles: func() error { return nil }}}
}

func TestClientRejectsAgentAndMissingServiceProof(t *testing.T) {
	for _, kind := range []authorityKind{0, authorityAgent} {
		server, conn := net.Pipe()
		client, err := NewClient(context.Background(), clientChannel(conn, kind), testLease())
		_ = server.Close()
		if client != nil || err != ErrAuthority {
			t.Fatal("non-service proof accepted by client")
		}
	}
	if client, err := NewClient(context.Background(), nil, testLease()); client != nil || err != ErrAuthority {
		t.Fatal("missing channel accepted")
	}
}

func TestClientWorkerRoundtripAndRecoveryUseOneLease(t *testing.T) {
	input := &recordingInput{}
	lease := testLease()
	conn, done := startWorker(t, context.Background(), input, lease)
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), lease)
	if err != nil {
		t.Fatal(err)
	}
	key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}
	for _, operation := range []func() error{
		func() error { return client.Press(context.Background(), key) },
		func() error { return client.Move(context.Background(), 1, 1) },
		func() error { return client.Wheel(context.Background(), 0, 120) },
		func() error { return client.Heartbeat(context.Background()) },
	} {
		if err := operation(); err != nil {
			t.Fatal("authenticated roundtrip failed")
		}
	}
	if err := client.Close(); err != nil {
		t.Fatal("client close failed")
	}
	result := await(t, done)
	if result.err != nil || result.report.Completed != 3 || result.report.Recovery.Remaining != 0 || input.pressed != 1 || input.released != 1 {
		t.Fatal("lease input/recovery mismatch")
	}
}

func TestClientLostAckIsUnknownAndNeverResent(t *testing.T) {
	server, conn := net.Pipe()
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); defer server.Close(); _, _ = readRequest(server) }()
	key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}
	err = client.Press(context.Background(), key)
	if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionOutcomeUnknown {
		t.Fatal("lost response treated as unapplied")
	}
	err = client.Press(context.Background(), key)
	if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionNotApplied {
		t.Fatal("failed channel reused")
	}
	<-done
}

func TestClientCancellationInterruptsAckWait(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	read := make(chan struct{})
	go func() { _, _ = readRequest(server); close(read) }()
	done := make(chan error, 1)
	go func() { done <- client.Move(ctx, 1, 0) }()
	<-read
	cancel()
	select {
	case err := <-done:
		if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionOutcomeUnknown {
			t.Fatal("in-flight cancellation treated as unapplied")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled response wait remained blocked")
	}
}

func TestClientRejectsReplayAckAndReportsWorkerInjectionFailure(t *testing.T) {
	for _, example := range []struct {
		ack  response
		want application.InjectionCertainty
	}{
		{response{op: opPress, sequence: 2}, application.InjectionOutcomeUnknown},
		{response{op: opPress, sequence: 1, status: 1, certainty: 1, requested: 1}, application.InjectionNotApplied},
		{response{op: opPress, sequence: 1, status: 1, certainty: 2, requested: 2, inserted: 1}, application.InjectionPartiallyApplied},
	} {
		server, conn := net.Pipe()
		client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer server.Close()
			_, _ = readRequest(server)
			_ = writeResponse(server, example.ack)
		}()
		err = client.Press(context.Background(), control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65})
		if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != example.want {
			t.Fatal("untrusted acknowledgement certainty")
		}
		_ = client.Close()
		<-done
	}
}

func TestResponseDecoderRejectsMalformedCertaintyAndReservedBytes(t *testing.T) {
	var encoded bytes.Buffer
	if writeResponse(&encoded, response{op: opPress, sequence: 1}) != nil {
		t.Fatal("test ack encode failed")
	}
	var base [responseSize]byte
	copy(base[:], encoded.Bytes())
	for _, mutate := range []func(*[responseSize]byte){
		func(b *[responseSize]byte) { b[20] = 1 },
		func(b *[responseSize]byte) { b[6] = 2 },
		func(b *[responseSize]byte) { b[6], b[7] = 1, 1; binary.BigEndian.PutUint16(b[18:20], 1) },
		func(b *[responseSize]byte) {
			b[6], b[7] = 1, 2
			binary.BigEndian.PutUint16(b[16:18], 1)
			binary.BigEndian.PutUint16(b[18:20], 1)
		},
	} {
		data := base
		mutate(&data)
		if _, err := decodeResponse(data); !errors.Is(err, ErrProtocol) {
			t.Fatal("malformed reply accepted")
		}
	}
}
