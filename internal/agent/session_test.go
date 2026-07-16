package agent

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
)

func TestRunSessionWiresBoundaryInputReturnAndLocalCleanup(t *testing.T) {
	leftConnection, rightConnection := openConnectionPair(t)
	leftHook := &triggerHook{returned: make(chan struct{})}
	rightHook := &idleHook{}
	leftInput := newTestInput(false)
	rightInput := newTestInput(true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 2)
	config := DefaultConfig(control.BoundaryConfig{
		Edge: control.ScreenEdgeRight, ScreenLeft: 0, ScreenWidth: 100,
		ActivationGap: 0, RearmDistance: 10,
	})
	go func() {
		result, err := RunSession(ctx, config, Dependencies{
			Connection: leftConnection, Hook: leftHook, Input: leftInput,
			Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		})
		done <- outcome{result: result, err: err}
	}()
	go func() {
		result, err := RunSession(ctx, config, Dependencies{
			Connection: rightConnection, Hook: rightHook, Input: rightInput,
			Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
		})
		done <- outcome{result: result, err: err}
	}()

	select {
	case move := <-rightInput.moves:
		if move != [2]int32{7, -3} {
			t.Fatalf("destination move = %v", move)
		}
	case <-ctx.Done():
		t.Fatalf("destination input timeout: %v", ctx.Err())
	}
	select {
	case <-leftHook.returned:
	case <-ctx.Done():
		t.Fatalf("source return timeout: %v", ctx.Err())
	}
	cancel()
	for range 2 {
		select {
		case finished := <-done:
			if !errors.Is(finished.err, context.Canceled) {
				t.Fatalf("RunSession() error = %v", finished.err)
			}
			if finished.result.FinalState != control.StateLocal {
				t.Fatalf("final state = %s", finished.result.FinalState)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("agent session did not stop")
		}
	}
}

func TestRunSessionRejectsIncompleteDependencies(t *testing.T) {
	_, err := RunSession(context.Background(), Config{}, Dependencies{})
	if err == nil {
		t.Fatal("RunSession() accepted incomplete dependencies")
	}
}

func TestSessionFailureComponentReturnsOnlyStableComponent(t *testing.T) {
	privateCause := errors.New("private wheel delta")
	err := &sessionComponentError{component: "source-session", cause: privateCause}
	if got := SessionFailureComponent(err); got != "source-session" {
		t.Fatalf("component = %q", got)
	}
	if !errors.Is(err, privateCause) {
		t.Fatal("component error did not preserve internal cause")
	}
	if got := SessionFailureComponent(privateCause); got != "unknown" {
		t.Fatalf("generic component = %q", got)
	}
}

func TestObservedSourceSinkCountsOnlySuccessfulSends(t *testing.T) {
	observer := &recordingSourceObserver{}
	delegate := &recordingSourceSink{}
	sink := &observedSourceSink{delegate: delegate, observer: observer}
	action := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	if err := sink.SendAction(context.Background(), action); err != nil {
		t.Fatalf("SendAction() error = %v", err)
	}
	if observer.actions != 1 {
		t.Fatalf("observed actions = %d, want 1", observer.actions)
	}
	delegate.sendErr = errors.New("send failed")
	if err := sink.SendAction(context.Background(), action); !errors.Is(err, delegate.sendErr) {
		t.Fatalf("failed SendAction() error = %v", err)
	}
	if observer.actions != 1 {
		t.Fatalf("failed send changed observed actions to %d", observer.actions)
	}
}

func TestCombinedSourceActivationOrdersCaptureLifecycleAndRollsBackFailure(t *testing.T) {
	var order []string
	source := &recordingActivation{name: "source", order: &order}
	capture := &recordingActivation{name: "capture", order: &order}
	activation := &combinedSourceActivation{source: source, capture: capture}
	if err := activation.Activate(); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	activation.Deactivate()
	if got := strings.Join(order, ","); got != "source.activate,capture.activate,capture.deactivate,source.deactivate" {
		t.Fatalf("activation order = %q", got)
	}

	order = nil
	source = &recordingActivation{name: "source", order: &order}
	capture = &recordingActivation{name: "capture", order: &order, activateErr: errors.New("foreground denied")}
	activation = &combinedSourceActivation{source: source, capture: capture}
	if err := activation.Activate(); !errors.Is(err, capture.activateErr) {
		t.Fatalf("Activate() error = %v", err)
	}
	if got := strings.Join(order, ","); got != "source.activate,capture.activate,source.deactivate,capture.deactivate" {
		t.Fatalf("rollback order = %q", got)
	}
	if activation.Active() {
		t.Fatal("activation remained active after capture failure")
	}
}

func TestRunSessionTreatsIndependentComponentContextErrorAsFailure(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "canceled", err: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			connection, peerConnection := openConnectionPair(t)
			defer peerConnection.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type outcome struct {
				result Result
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := RunSession(ctx, DefaultConfig(control.BoundaryConfig{
					Edge: control.ScreenEdgeRight, ScreenLeft: 0, ScreenWidth: 100,
					ActivationGap: 0, RearmDistance: 10,
				}), Dependencies{
					Connection: connection, Hook: contextErrorHook{err: test.err}, Input: newTestInput(false),
					Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
				})
				done <- outcome{result: result, err: err}
			}()

			select {
			case finished := <-done:
				if !errors.Is(finished.err, test.err) {
					t.Fatalf("RunSession() error = %v, want component error %v", finished.err, test.err)
				}
				if finished.result.FinalState != control.StateLocal {
					t.Fatalf("final state = %s", finished.result.FinalState)
				}
			case <-time.After(time.Second):
				cancel()
				select {
				case <-done:
				case <-time.After(time.Second):
				}
				t.Fatalf("independent component error %v did not stop agent session", test.err)
			}
		})
	}
}

type triggerHook struct {
	returned chan struct{}
}

func (hook *triggerHook) RunWithBoundary(ctx context.Context, session *wininput.SourceSession, boundary wininput.BoundaryObserver, ready func()) error {
	if ready != nil {
		ready()
	}
	if err := boundary.Observe(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 99); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !session.Active() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	if !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 7, DeltaY: -3}) {
		return errors.New("source action was not accepted")
	}
	for session.Active() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	close(hook.returned)
	<-ctx.Done()
	return ctx.Err()
}

type idleHook struct{}

func (*idleHook) RunWithBoundary(ctx context.Context, _ *wininput.SourceSession, _ wininput.BoundaryObserver, ready func()) error {
	if ready != nil {
		ready()
	}
	<-ctx.Done()
	return ctx.Err()
}

type contextErrorHook struct {
	err error
}

type recordingActivation struct {
	name        string
	order       *[]string
	active      bool
	activateErr error
}

func (activation *recordingActivation) Activate() error {
	*activation.order = append(*activation.order, activation.name+".activate")
	if activation.activateErr != nil {
		return activation.activateErr
	}
	activation.active = true
	return nil
}

func (activation *recordingActivation) Deactivate() {
	*activation.order = append(*activation.order, activation.name+".deactivate")
	activation.active = false
}

func (activation *recordingActivation) Active() bool { return activation.active }

func (hook contextErrorHook) RunWithBoundary(context.Context, *wininput.SourceSession, wininput.BoundaryObserver, func()) error {
	return hook.err
}

type testInput struct {
	mu         sync.Mutex
	intent     control.ReturnIntent
	returnNext bool
	returns    chan control.ReturnIntent
	moves      chan [2]int32
}

func newTestInput(returnNext bool) *testInput {
	return &testInput{returnNext: returnNext, returns: make(chan control.ReturnIntent, 1), moves: make(chan [2]int32, 1)}
}

func (*testInput) Press(context.Context, control.PressedToken) error      { return nil }
func (*testInput) Release(context.Context, control.PressedToken) error    { return nil }
func (*testInput) Wheel(context.Context, int32, int32) error              { return nil }
func (*testInput) PrepareEntry(context.Context, control.ScreenEdge) error { return nil }
func (*testInput) CommitEntry()                                           {}
func (*testInput) RollbackEntry(context.Context) (application.CursorRollbackOutcome, error) {
	return application.CursorRollbackNotRestored, nil
}
func (input *testInput) Move(_ context.Context, x, y int32) error {
	input.moves <- [2]int32{x, y}
	input.mu.Lock()
	intent := input.intent
	shouldReturn := input.returnNext
	input.returnNext = false
	input.mu.Unlock()
	if shouldReturn {
		input.returns <- intent
	}
	return nil
}
func (input *testInput) ArmReturn(intent control.ReturnIntent, _ control.ScreenEdge) error {
	input.mu.Lock()
	input.intent = intent
	input.mu.Unlock()
	return nil
}
func (*testInput) RearmReturn(control.ReturnIntent) {}
func (input *testInput) DisarmReturn(control.ReturnIntent) {
	input.mu.Lock()
	input.intent = control.ReturnIntent{}
	input.mu.Unlock()
}
func (input *testInput) ReturnIntents() <-chan control.ReturnIntent { return input.returns }

type noopEffects struct{}

func (noopEffects) Execute(context.Context, control.Effect) error { return nil }

type recordingSourceObserver struct {
	actions int
}

func (observer *recordingSourceObserver) ObserveSourceAction(control.InputAction) {
	observer.actions++
}

func (*recordingSourceObserver) ObserveBoundaryIntent(control.BoundaryIntent) {}

type recordingSourceSink struct {
	sendErr error
}

func (sink *recordingSourceSink) SendAction(context.Context, control.InputAction) error {
	return sink.sendErr
}

func (*recordingSourceSink) Interrupt(error) error             { return nil }
func (*recordingSourceSink) Fail(context.Context, error) error { return nil }

type acceptingAuthorizer struct{}

func (acceptingAuthorizer) AuthorizeLease(context.Context, transport.AuthenticatedPeerContext, string, string, time.Time) (string, bool, error) {
	return "lease-agent-test", true, nil
}

type framedConnection struct {
	net.Conn
	reader *transport.Reader
	writer *transport.Writer
	mu     sync.Mutex
}

func (*framedConnection) AuthenticatedPeer() (transport.AuthenticatedPeerContext, error) {
	return testAuthenticatedPeerContext(), nil
}

func testAuthenticatedPeerContext() transport.AuthenticatedPeerContext {
	return transport.AuthenticatedPeerContext{
		CertificateSHA256: strings.Repeat("b", 64),
		Role:              transport.AuthenticatedPeerConnector,
	}
}

func openConnectionPair(t *testing.T) (*framedConnection, *framedConnection) {
	t.Helper()
	left, right := net.Pipe()
	makeConnection := func(connection net.Conn) *framedConnection {
		reader, err := transport.NewReader(connection)
		if err != nil {
			t.Fatal(err)
		}
		writer, err := transport.NewWriter(connection)
		if err != nil {
			t.Fatal(err)
		}
		return &framedConnection{Conn: connection, reader: reader, writer: writer}
	}
	return makeConnection(left), makeConnection(right)
}

func (connection *framedConnection) ReadFrame() (transport.Frame, error) {
	return connection.reader.ReadFrame()
}
func (connection *framedConnection) WriteFrameContext(ctx context.Context, frame transport.Frame) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.writer.WriteFrame(frame)
}
