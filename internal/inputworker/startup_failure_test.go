package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"testing/iotest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestBootstrapFailureRetainsCauseAndCannotReachTransfer(t *testing.T) {
	for _, tc := range []struct {
		name           string
		reader         io.Reader
		checkErr, want error
	}{
		{"missing authentication", bytes.NewReader(nil), nil, io.EOF},
		{"truncated authentication", bytes.NewReader(workerAuthenticatedFrame[:3]), nil, io.ErrUnexpectedEOF},
		{"authentication timeout", iotest.ErrReader(os.ErrDeadlineExceeded), nil, os.ErrDeadlineExceeded},
		{"authentication native error", iotest.ErrReader(syscall.Errno(109)), nil, syscall.Errno(109)},
		{"authority rejection", bytes.NewReader(workerAuthenticatedFrame[:]), syscall.Errno(5), syscall.Errno(5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := bootstrapAfterAuthentication(context.Background(), tc.reader, func() error { return tc.checkErr }, func() ([bootstrapSize]byte, error) {
				t.Fatal("failed authentication transferred handles")
				return [bootstrapSize]byte{}, nil
			})
			if !errors.Is(err, ErrAuthority) || !errors.Is(err, tc.want) {
				t.Fatal("bootstrap erased original cause", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, err := bootstrapAfterAuthentication(ctx, bytes.NewReader(workerAuthenticatedFrame[:]), func() error { cancel(); return nil }, func() ([bootstrapSize]byte, error) {
		t.Fatal("cancellation transferred handles")
		return [bootstrapSize]byte{}, nil
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrAuthority) {
		t.Fatal("cancellation was erased", err)
	}
}

func TestStartupReadyRetainsFailureAcrossEveryEnvelopeBoundary(t *testing.T) {
	var wire bytes.Buffer
	if err := writeStartupReady(&wire, decisionProfile(handoff.PreparationUsed, true)); err != nil {
		t.Fatal(err)
	}
	packet := wire.Bytes()
	for _, length := range []int{0, 3, 8, startupReadySize - 1, startupReadySize, startupReadyLoadSize, startupReadyPreparedSize, startupReadyDecisionSize - 1} {
		want := io.EOF
		if length != 0 && length != 8 && length != startupReadySize && length != startupReadyLoadSize && length != startupReadyPreparedSize {
			want = io.ErrUnexpectedEOF
		}
		_, err := readStartupReady(bytes.NewReader(packet[:length]))
		if !errors.Is(err, ErrProtocol) || !errors.Is(err, want) {
			t.Fatal("truncated envelope lost I/O cause", length, err)
		}
	}
	for _, native := range []error{os.ErrDeadlineExceeded, net.ErrClosed, syscall.Errno(109)} {
		for _, prefix := range []int{0, 8, startupReadySize, startupReadyLoadSize, startupReadyPreparedSize} {
			_, err := readStartupReady(io.MultiReader(bytes.NewReader(packet[:prefix]), iotest.ErrReader(native)))
			if !errors.Is(err, ErrProtocol) || !errors.Is(err, native) {
				t.Fatal("READY erased transport failure", prefix, err)
			}
		}
	}
}

type startupPublishFixture struct {
	bytes.Buffer
	deadlines       int
	deadlineFailure int
	writeErr        error
	short           bool
}

func (f *startupPublishFixture) SetWriteDeadline(time.Time) error {
	f.deadlines++
	if f.deadlines == f.deadlineFailure {
		return net.ErrClosed
	}
	return nil
}
func (f *startupPublishFixture) Write(data []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.short {
		return len(data) - 1, nil
	}
	return f.Buffer.Write(data)
}

type startupPublishConn struct {
	net.Conn
	fixture *startupPublishFixture
}

func (c *startupPublishConn) Write(data []byte) (int, error) { return c.fixture.Write(data) }
func (c *startupPublishConn) SetWriteDeadline(at time.Time) error {
	return c.fixture.SetWriteDeadline(at)
}

func TestStartupPublicationStopsOnAuthorityDeadlineAndWriteFailures(t *testing.T) {
	for _, tc := range []struct {
		name               string
		denied             error
		fixture            startupPublishFixture
		want               error
		deadlines, written int
	}{
		{name: "denied", denied: syscall.Errno(5), want: syscall.Errno(5)},
		{name: "deadline", fixture: startupPublishFixture{deadlineFailure: 1}, want: net.ErrClosed, deadlines: 1},
		{name: "write", fixture: startupPublishFixture{writeErr: syscall.Errno(109)}, want: syscall.Errno(109), deadlines: 1},
		{name: "partial", fixture: startupPublishFixture{short: true}, want: io.ErrShortWrite, deadlines: 1},
		{name: "reset", fixture: startupPublishFixture{deadlineFailure: 2}, want: net.ErrClosed, deadlines: 2, written: 8},
		{name: "ready", deadlines: 2, written: 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer, pipe := net.Pipe()
			defer peer.Close()
			stream := &startupPublishConn{Conn: pipe, fixture: &tc.fixture}
			agent := clientChannel(stream, authorityAgent)
			defer agent.Close()
			calls := 0
			agent.authority.checkInput = func() error {
				calls++
				return tc.denied
			}
			agent.authority.checkRecovery = func() error { t.Error("recovery check authorized readiness"); return ErrAuthority }
			err := publishStartupReady(context.Background(), agent, time.Now().Add(time.Minute), handoff.StartupProfile{})
			if calls != 1 || tc.fixture.deadlines != tc.deadlines || tc.fixture.Len() != tc.written {
				t.Fatal("startup proceeded after rejection", calls, tc.fixture.deadlines, tc.fixture.Len())
			}
			if tc.want == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, ErrAuthority) || !errors.Is(err, tc.want) {
				t.Fatal("publication erased failure", err)
			}
		})
	}
}

func TestInputStartupRuntimeRetainsFixedFailureKindAndNativeCode(t *testing.T) {
	for _, tc := range []struct {
		cause  error
		kind   string
		native uint32
	}{
		{os.ErrDeadlineExceeded, "-timeout", 0}, {context.DeadlineExceeded, "-timeout", 0},
		{context.Canceled, "-cancelled", 0}, {io.EOF, "-eof", 0}, {io.ErrUnexpectedEOF, "-truncated", 0},
		{net.ErrClosed, "-closed", 0}, {syscall.Errno(109), "", 109},
	} {
		result := inputStartupFailure("ready", errors.Join(ErrAuthority, tc.cause))
		if !errors.Is(result, ErrAuthority) || !errors.Is(result, tc.cause) {
			t.Fatal("classification changed cause")
		}
		r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
		r.record("service", "FAILED", result)
		f := r.value.LastFailure
		if f == nil || f.Operation != "input-startup-ready"+tc.kind || f.Win32Error != tc.native || !r.value.Valid() {
			t.Fatal("runtime erased startup cause", f)
		}
	}
	specific := authorityCheckFailed("authority-process-query", syscall.Errno(5))
	var detail interface{ LoginRuntimeOperation() string }
	if result := inputStartupFailure("ready", errors.Join(specific, os.ErrDeadlineExceeded)); !errors.As(result, &detail) || detail.LoginRuntimeOperation() != "authority-process-query" {
		t.Fatal("startup replaced specific authority failure")
	}
}

func TestStartupFailureKeepsOriginalStageWhenCleanupAlsoFails(t *testing.T) {
	original := inputStartupFailure("ready", errors.Join(ErrAuthority, io.ErrUnexpectedEOF))
	cleanup := inputServiceAt("input-close-worker", errors.Join(ErrRecovery, syscall.Errno(6)))
	result := inputStartupFailure("ready", errors.Join(original, cleanup))
	var detail interface{ LoginRuntimeOperation() string }
	if !errors.Is(result, io.ErrUnexpectedEOF) || !errors.Is(result, ErrRecovery) || !errors.Is(result, syscall.Errno(6)) || !errors.As(result, &detail) || detail.LoginRuntimeOperation() != "input-startup-ready-truncated" {
		t.Fatal("cleanup replaced original startup failure or lost fence", result)
	}
}
