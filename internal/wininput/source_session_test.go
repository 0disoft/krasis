package wininput

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceSessionSendsAcceptedActionsInOrder(t *testing.T) {
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 2)}
	session := startSourceSession(t, 2, sink)
	if !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) ||
		!session.Offer(control.InputAction{Kind: control.InputWheel, DeltaY: 120}) {
		t.Fatal("valid actions were not accepted")
	}
	first := receiveAction(t, sink.sent)
	second := receiveAction(t, sink.sent)
	if first.Kind != control.InputPointerMove || second.Kind != control.InputWheel {
		t.Fatalf("send order = %s, %s", first.Kind, second.Kind)
	}
}

func TestSourceSessionDoesNotSuppressWhileInactive(t *testing.T) {
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 1)}
	session, err := NewSourceSession(1, sink)
	if err != nil {
		t.Fatalf("NewSourceSession() error = %v", err)
	}
	if session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
		t.Fatal("inactive source session accepted input")
	}
}

func TestSourceSessionReportsAcceptedAndRejectedWheelOffers(t *testing.T) {
	observer := &recordingSourcePipelineObserver{}
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 1)}
	session := startObservedSourceSession(t, 1, sink, observer)
	wheel := control.InputAction{Kind: control.InputWheel, DeltaY: 120}
	if !session.Offer(wheel) {
		t.Fatal("active wheel was not accepted")
	}
	if got := receiveAction(t, sink.sent); got != wheel {
		t.Fatalf("sent wheel = %+v", got)
	}
	session.Deactivate()
	if session.Offer(wheel) {
		t.Fatal("inactive wheel was accepted")
	}
	if observer.queueAccepted != 1 || observer.queueRejected != 1 || observer.rejectedInactive != 1 {
		t.Fatalf("pipeline observer = %+v", observer)
	}
	if observer.queueEnqueued != 1 || observer.queueDequeued != 1 || observer.residenceSamples != 1 || observer.maxQueueDepth != 1 {
		t.Fatalf("source queue observer = %+v", observer)
	}
}

func TestSourceSessionOverflowDisablesSuppressionAndFaults(t *testing.T) {
	blockSend := make(chan struct{})
	sink := &recordingSourceSink{blockSend: blockSend, sent: make(chan control.InputAction, 1), failed: make(chan error, 1)}
	observer := &recordingSourcePipelineObserver{}
	session := startObservedSourceSession(t, 1, sink, observer)
	wheel := control.InputAction{Kind: control.InputWheel, DeltaY: 120}
	if !session.Offer(wheel) {
		t.Fatal("first action was not accepted")
	}
	receiveAction(t, sink.sent)
	if !session.Offer(wheel) {
		t.Fatal("queued action was not accepted")
	}
	if session.Offer(wheel) {
		t.Fatal("overflowing action was incorrectly suppressed")
	}
	if observer.queueAccepted != 2 || observer.queueRejected != 1 || observer.rejectedOverflow != 1 {
		t.Fatalf("overflow pipeline observer = %+v", observer)
	}
	if session.Active() {
		t.Fatal("overflow did not disable suppression immediately")
	}
	close(blockSend)
	select {
	case err := <-sink.failed:
		if !errors.Is(err, ErrSourceQueueOverflow) {
			t.Fatalf("fault = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("source fault was not delivered")
	}
	select {
	case action := <-sink.sent:
		t.Fatalf("stale queued action was sent: %+v", action)
	case <-time.After(20 * time.Millisecond):
	}
	if observer.queueDiscardedStale != 1 {
		t.Fatalf("stale queue evidence = %+v", observer)
	}
}

func TestSourceSessionInvalidActionFailsOpen(t *testing.T) {
	sink := &recordingSourceSink{failed: make(chan error, 1)}
	observer := &recordingSourcePipelineObserver{}
	session := startObservedSourceSession(t, 1, sink, observer)
	if session.Offer(control.InputAction{Kind: control.InputWheel}) {
		t.Fatal("invalid action was suppressed")
	}
	if observer.queueRejected != 1 || observer.rejectedInvalid != 1 {
		t.Fatalf("invalid pipeline observer = %+v", observer)
	}
	if session.Active() {
		t.Fatal("invalid action left suppression active")
	}
	select {
	case err := <-sink.failed:
		if !errors.Is(err, control.ErrInvalidInputAction) {
			t.Fatalf("fault = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("invalid action fault was not delivered")
	}
}

func TestSourceSessionSendFailureDisablesSuppression(t *testing.T) {
	sendErr := errors.New("send failed")
	sink := &recordingSourceSink{sendErr: sendErr, failed: make(chan error, 1)}
	session := startSourceSession(t, 1, sink)
	if !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
		t.Fatal("action was not accepted")
	}
	select {
	case err := <-sink.failed:
		if !errors.Is(err, sendErr) {
			t.Fatalf("fault = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("send failure was not delivered")
	}
	if session.Active() {
		t.Fatal("send failure left suppression active")
	}
}

func TestSourceSessionIgnoresSendFailureFromRevokedGeneration(t *testing.T) {
	sink := &revokedGenerationSourceSink{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
		sent:         make(chan control.InputAction, 1),
		failed:       make(chan error, 1),
	}
	session := startSourceSession(t, 2, sink)
	first := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	if !session.Offer(first) {
		t.Fatal("first generation action was not accepted")
	}
	select {
	case <-sink.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first generation send did not start")
	}
	session.Deactivate()
	if err := session.Activate(); err != nil {
		t.Fatalf("reactivate source session: %v", err)
	}
	second := control.InputAction{Kind: control.InputWheel, DeltaY: 120}
	if !session.Offer(second) {
		t.Fatal("second generation action was not accepted")
	}
	close(sink.releaseFirst)
	if got := receiveAction(t, sink.sent); got != second {
		t.Fatalf("sent action = %+v, want %+v", got, second)
	}
	select {
	case err := <-sink.failed:
		t.Fatalf("revoked generation failed the source session: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestSourceSessionInterruptsBlockedSendBeforeFaultCleanup(t *testing.T) {
	sink := &interruptibleSourceSink{
		started:     make(chan struct{}),
		interrupted: make(chan struct{}),
		failed:      make(chan error, 1),
	}
	session := startSourceSession(t, 1, sink)
	action := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	if !session.Offer(action) {
		t.Fatal("first action was not accepted")
	}
	select {
	case <-sink.started:
	case <-time.After(time.Second):
		t.Fatal("SendAction did not start")
	}
	if !session.Offer(action) {
		t.Fatal("queued action was not accepted")
	}
	if session.Offer(action) {
		t.Fatal("overflowing action was incorrectly accepted")
	}
	select {
	case cause := <-sink.failed:
		if !errors.Is(cause, ErrSourceQueueOverflow) {
			t.Fatalf("cleanup cause = %v", cause)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked send prevented fault cleanup")
	}
}

func TestSourceSessionBoundsBlockingInterrupt(t *testing.T) {
	sink := &blockingInterruptSourceSink{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	session, err := NewSourceSession(1, sink)
	if err != nil {
		t.Fatalf("NewSourceSession() error = %v", err)
	}
	session.cleanup = 25 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- session.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for !session.started.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := session.Activate(); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	cancel()
	select {
	case <-sink.started:
	case <-time.After(time.Second):
		t.Fatal("Interrupt did not start")
	}
	select {
	case runErr := <-done:
		if !errors.Is(runErr, context.DeadlineExceeded) {
			t.Fatalf("Run() error = %v", runErr)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("blocking Interrupt escaped the cleanup deadline")
	}
	close(sink.release)
}

func startSourceSession(t *testing.T, capacity int, sink SourceSink) *SourceSession {
	return startObservedSourceSession(t, capacity, sink, nil)
}

func startObservedSourceSession(t *testing.T, capacity int, sink SourceSink, observer SourcePipelineObserver) *SourceSession {
	t.Helper()
	session, err := NewSourceSessionObserved(capacity, sink, observer)
	if err != nil {
		t.Fatalf("NewSourceSession() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- session.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("source session did not stop")
		}
	})
	deadline := time.Now().Add(time.Second)
	for !session.started.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := session.Activate(); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	return session
}

type recordingSourcePipelineObserver struct {
	hookObserved         uint64
	synthesizedObserved  uint64
	rawObserved          uint64
	windowObserved       uint64
	captureActivations   uint64
	captureDeactivations uint64
	foregroundFallbacks  []SourceForegroundFallbackResult
	focusFallbacks       []SourceFocusRestoreFallbackResult
	focusRestoreResults  []SourceFocusRestoreResult
	postHideFocusResults []SourcePostHideFocusResult
	activeHookMoves      uint64
	activeHookButtons    uint64
	mapped               uint64
	queueAccepted        uint64
	queueRejected        uint64
	rejectedInactive     uint64
	rejectedInvalid      uint64
	rejectedOverflow     uint64
	queueEnqueued        uint64
	queueDequeued        uint64
	queueDiscardedStale  uint64
	maxQueueDepth        uint64
	residenceSamples     uint64
	maxResidence         time.Duration
}

func (observer *recordingSourcePipelineObserver) ObserveWheelHook() {
	observer.hookObserved++
}

func (observer *recordingSourcePipelineObserver) ObserveWheelSynthesized() {
	observer.synthesizedObserved++
}

func (observer *recordingSourcePipelineObserver) ObserveWheelRaw() {
	observer.rawObserved++
}

func (observer *recordingSourcePipelineObserver) ObserveWheelWindow() {
	observer.windowObserved++
}

func (observer *recordingSourcePipelineObserver) ObserveWheelCaptureActivated() {
	observer.captureActivations++
}

func (observer *recordingSourcePipelineObserver) ObserveWheelCaptureDeactivated() {
	observer.captureDeactivations++
}

func (observer *recordingSourcePipelineObserver) ObserveWheelForegroundFallback(result SourceForegroundFallbackResult) {
	observer.foregroundFallbacks = append(observer.foregroundFallbacks, result)
}

func (observer *recordingSourcePipelineObserver) ObserveWheelFocusRestoreFallback(result SourceFocusRestoreFallbackResult) {
	observer.focusFallbacks = append(observer.focusFallbacks, result)
}

func (observer *recordingSourcePipelineObserver) ObserveWheelFocusRestore(result SourceFocusRestoreResult) {
	observer.focusRestoreResults = append(observer.focusRestoreResults, result)
}

func (observer *recordingSourcePipelineObserver) ObserveWheelPostHideFocus(result SourcePostHideFocusResult) {
	observer.postHideFocusResults = append(observer.postHideFocusResults, result)
}

func (observer *recordingSourcePipelineObserver) ObserveActiveHookMouseMove() {
	observer.activeHookMoves++
}

func (observer *recordingSourcePipelineObserver) ObserveActiveHookMouseButton() {
	observer.activeHookButtons++
}

func (observer *recordingSourcePipelineObserver) ObserveWheelMapped() {
	observer.mapped++
}

func (observer *recordingSourcePipelineObserver) ObserveWheelQueue(result SourceQueueOfferResult) {
	switch result {
	case SourceQueueAccepted:
		observer.queueAccepted++
	case SourceQueueRejectedInactive:
		observer.queueRejected++
		observer.rejectedInactive++
	case SourceQueueRejectedInvalid:
		observer.queueRejected++
		observer.rejectedInvalid++
	case SourceQueueRejectedOverflow:
		observer.queueRejected++
		observer.rejectedOverflow++
	}
}

func (observer *recordingSourcePipelineObserver) ObserveSourceQueueEnqueued(depth uint64) {
	observer.queueEnqueued++
	if depth > observer.maxQueueDepth {
		observer.maxQueueDepth = depth
	}
}

func (observer *recordingSourcePipelineObserver) ObserveSourceQueueDequeued(residence time.Duration) {
	observer.queueDequeued++
	observer.residenceSamples++
	if residence > observer.maxResidence {
		observer.maxResidence = residence
	}
}

func (observer *recordingSourcePipelineObserver) ObserveSourceQueueDiscardedStale() {
	observer.queueDiscardedStale++
}

func receiveAction(t *testing.T, actions <-chan control.InputAction) control.InputAction {
	t.Helper()
	select {
	case action := <-actions:
		return action
	case <-time.After(time.Second):
		t.Fatal("action was not sent")
		return control.InputAction{}
	}
}

type recordingSourceSink struct {
	sent      chan control.InputAction
	failed    chan error
	blockSend <-chan struct{}
	sendErr   error
}

type blockingInterruptSourceSink struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (*blockingInterruptSourceSink) SendAction(ctx context.Context, _ control.InputAction) error {
	<-ctx.Done()
	return ctx.Err()
}

func (sink *blockingInterruptSourceSink) Interrupt(error) error {
	sink.once.Do(func() { close(sink.started) })
	<-sink.release
	return nil
}

func (*blockingInterruptSourceSink) Fail(context.Context, error) error { return nil }

func (sink *recordingSourceSink) SendAction(_ context.Context, action control.InputAction) error {
	if sink.sent != nil {
		sink.sent <- action
	}
	if sink.blockSend != nil {
		<-sink.blockSend
	}
	return sink.sendErr
}

func (*recordingSourceSink) Interrupt(error) error { return nil }

func (sink *recordingSourceSink) Fail(_ context.Context, cause error) error {
	if sink.failed != nil {
		sink.failed <- cause
	}
	return nil
}

type interruptibleSourceSink struct {
	startedOnce   sync.Once
	interruptOnce sync.Once
	started       chan struct{}
	interrupted   chan struct{}
	failed        chan error
}

type revokedGenerationSourceSink struct {
	mu           sync.Mutex
	calls        int
	firstStarted chan struct{}
	releaseFirst chan struct{}
	sent         chan control.InputAction
	failed       chan error
}

func (sink *revokedGenerationSourceSink) SendAction(_ context.Context, action control.InputAction) error {
	sink.mu.Lock()
	sink.calls++
	call := sink.calls
	sink.mu.Unlock()
	if call == 1 {
		close(sink.firstStarted)
		<-sink.releaseFirst
		return errors.New("revoked generation send failed")
	}
	sink.sent <- action
	return nil
}

func (*revokedGenerationSourceSink) Interrupt(error) error { return nil }

func (sink *revokedGenerationSourceSink) Fail(_ context.Context, cause error) error {
	sink.failed <- cause
	return nil
}

func (sink *interruptibleSourceSink) SendAction(context.Context, control.InputAction) error {
	sink.startedOnce.Do(func() { close(sink.started) })
	<-sink.interrupted
	return errors.New("send interrupted")
}

func (sink *interruptibleSourceSink) Interrupt(error) error {
	sink.interruptOnce.Do(func() { close(sink.interrupted) })
	return nil
}

func (sink *interruptibleSourceSink) Fail(_ context.Context, cause error) error {
	sink.failed <- cause
	return nil
}
