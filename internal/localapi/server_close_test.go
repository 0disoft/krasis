package localapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestServerCloseBeforeRunReleasesListener(t *testing.T) {
	server, err := Listen(filepath.Join(protectedRoot(t), "control"), &stubOperations{})
	if err != nil {
		t.Fatal(err)
	}
	// Also release a leaked listener when running this test against old code.
	defer server.listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatal(err)
	}
	address := server.listener.Addr().String()
	replacement, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("Close before Run left its TCP port bound: %v", err)
	}
	defer replacement.Close()
	// A late Run must not reopen the already retired endpoint, and repeated
	// Close must preserve success rather than surfacing net.ErrClosed.
	if err := server.Run(ctx); err != nil {
		t.Fatalf("Run after Close: %v", err)
	}
	if err := server.Close(ctx); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
}

func TestServerCloseBeforeRunPreservesListenerFailure(t *testing.T) {
	want := errors.New("listener retirement failed")
	listener := &retirementFailureListener{err: want}
	server := &Server{listener: listener, http: &http.Server{}}
	const callers = 8
	results := make(chan error, callers)
	for range callers {
		go func() { results <- server.Close(context.Background()) }()
	}
	for range callers {
		if err := <-results; !errors.Is(err, want) {
			t.Fatalf("Close error = %v, want %v", err, want)
		}
	}
	if listener.calls != 1 {
		t.Fatalf("listener closed %d times, want one", listener.calls)
	}
}

type retirementFailureListener struct {
	net.Listener
	err   error
	calls int
}

func (l *retirementFailureListener) Close() error {
	l.calls++
	return l.err
}
