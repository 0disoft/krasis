package peerupdate

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancelledDiagnosticExportRetainsSlotAndRejectsLatePublication(t *testing.T) {
	m := testManager(t)
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	archive := queuedFileOffer(t, m, peer)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	finish := func() { releaseOnce.Do(func() { close(release) }) }
	defer finish()
	var calls atomic.Int32
	events := make(chan string, 8)
	m.Observe = func(code string) { events <- code }
	m.ExportDiagnostic = func(p, request string) error {
		calls.Add(1)
		close(entered)
		<-release // Model a native/filesystem call that ignores cancellation.
		return m.QueueDiagnosticForRequest(p, request, []byte(`{}`))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- m.runDiagnosticExport(ctx, peer, id, func() bool { return true }) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("export did not start")
	}
	m.diagnostic.mu.Lock()
	job := m.diagnostic.export
	m.diagnostic.mu.Unlock()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("export lost cancellation cause", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled caller waited for storage")
	}
	if err := m.runDiagnosticExport(context.Background(), peer, id, func() bool { return true }); !errors.Is(err, context.Canceled) {
		t.Fatal("same request started another exporter", err)
	}
	if err := m.runDiagnosticExport(context.Background(), peer, strings.Repeat("c", 32), func() bool { return true }); !errors.Is(err, ErrBusy) {
		t.Fatal("new request bypassed occupied export slot", err)
	}
	response := httptest.NewRecorder()
	m.handler(peer, func() bool { return true }).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/bundle", nil))
	if response.Code != 200 || int64(response.Body.Len()) != archive.size {
		t.Fatal("stuck export blocked unrelated bundle work", response.Code)
	}
	finish()
	select {
	case <-job.done:
	case <-time.After(2 * time.Second):
		t.Fatal("export slot was not released")
	}
	if calls.Load() != 1 || !errors.Is(job.err, context.Canceled) || m.diagnosticOffer(peer) != nil {
		t.Fatal("cancelled export published a late result or ran twice", calls.Load(), job.err)
	}
	if code := <-events; code != "peer_diagnostic_export_cancelled" {
		t.Fatal("export failure was swallowed", code)
	}
}

func TestDiagnosticExportStillExpiresPreviouslyHandledOffer(t *testing.T) {
	m := testManager(t)
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	if err := m.QueueDiagnosticForRequest(peer, id, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	m.diagnostic.out.expires = time.Now().Add(-time.Second)
	cause := errors.New("new export observed expired receipt")
	m.ExportDiagnostic = func(string, string) error { return cause }
	if err := m.runDiagnosticExport(context.Background(), peer, id, func() bool { return true }); !errors.Is(err, cause) {
		t.Fatal("expired offer was still treated as handled", err)
	}
}

func TestDiagnosticExportPreservesCauseAndUsesContextCallback(t *testing.T) {
	m := testManager(t)
	cause := errors.New("private storage failure")
	m.ExportDiagnostic = func(string, string) error { return errors.New("wrong callback") }
	m.ExportDiagnosticContext = func(ctx context.Context, _, _ string) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("export has no deadline")
		}
		return cause
	}
	if err := m.runDiagnosticExport(context.Background(), strings.Repeat("a", 64), strings.Repeat("b", 32), func() bool { return true }); !errors.Is(err, cause) {
		t.Fatal("export cause was lost", err)
	}
}

func TestCompletedDiagnosticExportSurvivesCleanupCancellation(t *testing.T) {
	for _, result := range []error{nil, errors.New("storage failure")} {
		run, cancel := context.WithCancel(context.Background())
		job := &diagnosticExport{ctx: run, done: make(chan struct{}), err: result}
		close(job.done)
		cancel() // Export worker cleanup after publishing its result.
		for range 32 {
			if err := waitDiagnosticExport(context.Background(), job); !errors.Is(err, result) {
				t.Fatalf("completed export returned %v, want %v", err, result)
			}
		}
	}
}

func TestPendingDiagnosticExportStillReturnsCancellation(t *testing.T) {
	run, cancel := context.WithCancel(context.Background())
	job := &diagnosticExport{ctx: run, done: make(chan struct{})}
	cancel()
	if err := waitDiagnosticExport(context.Background(), job); !errors.Is(err, context.Canceled) {
		t.Fatalf("pending export ignored cancellation: %v", err)
	}
}

func TestManagerCloseCancelsDiagnosticExport(t *testing.T) {
	m := testManager(t)
	entered := make(chan struct{})
	m.ExportDiagnosticContext = func(ctx context.Context, _, _ string) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	result := make(chan error, 1)
	go func() {
		result <- m.runDiagnosticExport(context.Background(), strings.Repeat("a", 64), strings.Repeat("b", 32), func() bool { return true })
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("export did not start")
	}
	if err := m.Close(); err != nil {
		t.Fatal("shutdown did not cancel its exporter", err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal("shutdown lost cancellation", err)
	}
}

func TestDiagnosticNetworkWaitCannotBlockBundleLane(t *testing.T) {
	local, remote := testManager(t), testManager(t)
	peer, caller := strings.Repeat("a", 64), strings.Repeat("b", 64)
	if err := local.Permit(peer, true); err != nil {
		t.Fatal(err)
	}
	if _, err := local.QueueDiagnosticRequest(peer, "local.json"); err != nil {
		t.Fatal(err)
	}
	entered, release, bundle := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce, bundleOnce sync.Once
	finish := func() { releaseOnce.Do(func() { close(release) }) }
	defer finish()
	remote.ExportDiagnostic = func(string, string) error { close(entered); <-release; return nil }
	handler := remote.handler(caller, func() bool { return true })
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/bundle" && r.Method == http.MethodGet {
			select {
			case <-entered:
				bundleOnce.Do(func() { close(bundle) })
				w.WriteHeader(http.StatusNoContent)
			case <-r.Context().Done():
			}
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "https://"))
	n, _ := strconv.Atoi(port)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		local.Run(ctx, net.JoinHostPort(host, strconv.Itoa(n-1)), peer, false, server.Client().Transport.(*http.Transport).TLSClientConfig, func() bool { return true })
	}()
	defer func() {
		cancel()
		finish()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("management workers failed to stop")
		}
	}()
	select {
	case <-bundle:
	case <-time.After(7 * time.Second):
		t.Fatal("diagnostic request occupied the bundle transport or loop")
	}
}
