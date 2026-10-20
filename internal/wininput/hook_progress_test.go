package wininput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestHookProgressIgnoresIdleAndShortRawBursts(t *testing.T) {
	base := time.Unix(100, 0)
	p := &sourceHookProgress{origin: base}
	for _, at := range []time.Time{base, base.Add(time.Hour)} {
		if err := p.failure(at); err != nil {
			t.Fatal("idle became hook failure", err)
		}
	}
	p.observePhysicalRaw(base.Add(2 * time.Hour))
	if err := p.failure(base.Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := p.failure(base.Add(2*time.Hour + time.Second)); err != nil {
		t.Fatal("completed burst became failure", err)
	}
	p.observePhysicalRaw(base.Add(3 * time.Hour))
	if err := p.failure(base.Add(3 * time.Hour)); err != nil {
		t.Fatal("first input after idle failed", err)
	}
}

func TestHookProgressRequiresSustainedPhysicalRawWithoutMouseHook(t *testing.T) {
	for _, healthy := range []bool{false, true} {
		base := time.Unix(100, 0)
		p := &sourceHookProgress{origin: base}
		for step := 0; step <= 30; step++ {
			at := base.Add(time.Duration(step) * 100 * time.Millisecond)
			p.observePhysicalRaw(at)
			if healthy {
				p.observeMouse()
			}
			err := p.failure(at)
			if !healthy && step >= 20 {
				if !errors.Is(err, errHookMouseProgress) || SourceFailureStage(err) != "hook-mouse-progress" {
					t.Fatal("mismatch was not classified", err)
				}
			} else if err != nil {
				t.Fatal("healthy/short interval became failure", err)
			}
		}
	}
}

func TestHookProgressStalledCallbackDoesNotNeedRawInput(t *testing.T) {
	base := time.Unix(100, 0)
	p := &sourceHookProgress{origin: base}
	p.ObserveHookStarted(base)
	if err := p.failure(base.Add(999 * time.Millisecond)); err != nil {
		t.Fatal("callback failed too early", err)
	}
	if err := p.failure(base.Add(time.Second)); !errors.Is(err, errHookCallbackStalled) || SourceFailureStage(err) != "hook-callback-stalled" {
		t.Fatal(err)
	}
	p.ObserveHookCompleted(base.Add(time.Second))
	if err := p.failure(base.Add(time.Hour)); err != nil {
		t.Fatal("completed callback stayed stalled", err)
	}
	if n := testing.AllocsPerRun(100, func() {
		p.ObserveHookStarted(base)
		p.observeMouse()
		p.observePhysicalRaw(base)
		p.ObserveHookCompleted(base)
	}); n != 0 {
		t.Fatal("hook observation allocates", n)
	}
}

type blockedHookTiming struct{ entered, release chan struct{} }

func (b *blockedHookTiming) ObserveHookStarted(time.Time) { close(b.entered); <-b.release }
func (*blockedHookTiming) ObserveHookCompleted(time.Time) {}
func (*blockedHookTiming) ObserveRawInput(time.Time)      {}

func TestHookProgressDetectsBlockedObserverOutsideItsThread(t *testing.T) {
	base := time.Now()
	b := &blockedHookTiming{entered: make(chan struct{}), release: make(chan struct{})}
	p := &sourceHookProgress{origin: base, next: b}
	done := make(chan struct{})
	go func() { timedSourceCallback(func(int, uintptr, uintptr) uintptr { return 1 }, p)(0, 0, 0); close(done) }()
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		close(b.release)
		t.Fatal("observer did not enter")
	}
	err := p.failure(base.Add(2 * time.Second))
	close(b.release)
	<-done
	if !errors.Is(err, errHookCallbackStalled) {
		t.Fatal("observer delay escaped watchdog", err)
	}
}

func TestHookProgressRetirementFencesSuppressionWithoutWaitingForStateMutex(t *testing.T) {
	session, err := NewSourceSession(1, &recordingSourceSink{})
	if err != nil {
		t.Fatal(err)
	}
	session.active = true
	session.started.Store(true)
	session.generation = 4
	session.stateMu.Lock()
	defer session.stateMu.Unlock()
	session.retireHookFailure(errHookCallbackStalled)
	if session.Active() || session.Offer(control.InputAction{Kind: control.InputKeyDown, Code: 30}) || session.current(4) || session.Activate() == nil {
		t.Fatal("retired hook kept suppression or admitted a lease")
	}
	if err := <-session.faults; err != errHookCallbackStalled {
		t.Fatal("failure reason lost", err)
	}
}

func TestHookProgressRetirementProofRejectsUnjoinedOrFailedCleanup(t *testing.T) {
	progress := sourceStage("hook-mouse-progress", errHookMouseProgress)
	proof := &retiredHookProgress{cause: errors.Join(progress, context.Canceled)}
	if !SourceHookProgressRetiredOnly(proof) || SourceHookProgressRetiredOnly(progress) {
		t.Fatal("progress became cleanup proof")
	}
	for _, err := range []error{ErrSourceCleanupIncomplete, errors.New("unhook failed"), context.DeadlineExceeded} {
		if SourceHookProgressRetiredOnly(errors.Join(proof, err)) || SourceHookProgressFailureOnly(errors.Join(progress, err)) || SourceHookProgressRetiredOnly(&retiredHookProgress{cause: errors.Join(progress, err)}) {
			t.Fatal("cleanup uncertainty erased", err)
		}
	}
}

func TestHookProgressRetirementKeepsFailureWhenOwnerCancellationRaces(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		ctx, cancel := context.WithCancel(context.Background())
		sink := &recordingSourceSink{failed: make(chan error, 1)}
		session, err := NewSourceSession(1, sink)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		ready, done := make(chan struct{}), make(chan error, 1)
		go func() { done <- session.RunWithReady(ctx, func() { close(ready) }) }()
		<-ready
		if err := session.Activate(); err != nil {
			cancel()
			t.Fatal(err)
		}
		session.retireHookFailure(errHookCallbackStalled)
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, errHookCallbackStalled) {
				t.Fatal("cancellation erased hook failure", err)
			}
		case <-time.After(time.Second):
			t.Fatal("source worker did not finish")
		}
		select {
		case cause := <-sink.failed:
			if !errors.Is(cause, errHookCallbackStalled) {
				t.Fatal(cause)
			}
		default:
			t.Fatal("hook failure skipped held-input cleanup")
		}
	}
}
