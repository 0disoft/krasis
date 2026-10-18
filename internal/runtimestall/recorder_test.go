package runtimestall

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productlog"
)

type fakeSource struct {
	payload   string
	oversized bool
	stopped   atomic.Bool
}

func (*fakeSource) Start() error { return nil }
func (f *fakeSource) Stop()      { f.stopped.Store(true) }
func (f *fakeSource) WriteTo(w io.Writer) (int64, error) {
	if f.oversized {
		return io.CopyN(w, infiniteZero{}, maxSnapshotBytes+1)
	}
	return io.Copy(w, strings.NewReader(f.payload))
}

type infiniteZero struct{}

func (infiniteZero) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestTriggerIsNonblockingAndOnlyOneSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeSource{}
	entered, release := make(chan struct{}), make(chan struct{})
	var writes atomic.Int32
	r := start(ctx, f, func(traceSource) error { writes.Add(1); close(entered); <-release; return nil }, func(string) {}, time.Minute)
	var callers sync.WaitGroup
	for i := 0; i < 32; i++ {
		callers.Add(1)
		go func() { defer callers.Done(); r.Request() }()
	}
	joined := make(chan struct{})
	go func() { callers.Wait(); close(joined) }()
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("trigger blocked on snapshot")
	}
	<-entered
	close(release)
	<-r.done
	r.Close()
	if writes.Load() != 1 || !f.stopped.Load() {
		t.Fatal("snapshot was not single-use with cleanup")
	}
}

func TestExpiryStopsWithoutWriting(t *testing.T) {
	f := &fakeSource{}
	r := start(context.Background(), f, func(traceSource) error { t.Error("expired recorder wrote a trace"); return nil }, func(string) {}, time.Millisecond)
	<-r.done
	if !f.stopped.Load() {
		t.Fatal("recorder still active")
	}
	r.Close()
}

func TestFailedOrOversizedSnapshotPreservesPreviousAndNeverExports(t *testing.T) {
	base := t.TempDir()
	if err := save(base, &fakeSource{payload: "private-runtime-record"}); err != nil {
		t.Fatal(err)
	}
	if err := save(base, &fakeSource{oversized: true}); !errors.Is(err, errTraceLimit) {
		t.Fatal(err)
	}
	path := filepath.Join(base, "Krasis", "diagnostics", FileName)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "private-runtime-record" {
		t.Fatal("previous complete trace lost", err)
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".runtime-trace-*"))
	if err != nil || len(files) != 0 {
		t.Fatal("partial trace remains", files, err)
	}
	exported, err := productlog.Export(base)
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(exported)
	if err != nil || bytes.Contains(data, []byte("private-runtime-record")) || bytes.Contains(data, []byte(FileName)) {
		t.Fatal("raw trace entered support export", err)
	}
}

func TestOptInRealRecorderWritesLocalTrace(t *testing.T) {
	t.Setenv(Environment, "")
	if Start(context.Background(), t.TempDir(), func(string) {}) != nil {
		t.Fatal("default enabled")
	}
	t.Setenv(Environment, "1")
	base := t.TempDir()
	saved := make(chan struct{}, 1)
	r := Start(context.Background(), base, func(code string) {
		if code == "runtime_trace_saved" {
			saved <- struct{}{}
		}
	})
	if r == nil {
		t.Fatal("recorder did not start")
	}
	defer r.Close()
	time.Sleep(20 * time.Millisecond)
	r.Request()
	select {
	case <-saved:
	case <-time.After(5 * time.Second):
		t.Fatal("no trace saved")
	}
	data, err := os.ReadFile(filepath.Join(base, "Krasis", "diagnostics", FileName))
	if err != nil || len(data) < 16 || len(data) > maxSnapshotBytes || !bytes.HasPrefix(data, []byte("go 1.")) {
		t.Fatal("invalid trace", len(data), err)
	}
}
