package agent

import (
	"context"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
	"testing"
	"time"
)

type heightCapture struct {
	recordingActivation
	height control.EdgeHeight
}

func (c *heightCapture) DeactivateProfile(ctx context.Context) {
	c.height = control.EdgeHeightFromContext(ctx)
	c.Deactivate()
}

func TestCombinedCaptureCarriesReturnHeightToExactNativeOwner(t *testing.T) {
	var order []string
	c := &heightCapture{recordingActivation: recordingActivation{name: "capture", order: &order}}
	a := &combinedSourceActivation{source: &recordingActivation{name: "source", order: &order}, capture: c}
	a.DeactivateProfile(control.WithEdgeHeight(context.Background(), 65536))
	if c.height != 65536 || len(order) != 2 || order[0] != "source.deactivate" {
		t.Fatal("lost return height or suppression cleanup order", c.height, order)
	}
	// An ordinary shutdown/abort must not reuse a previous return position.
	a.Deactivate()
	if c.height != 0 {
		t.Fatal("return height leaked into later cleanup")
	}
}

type heightConnection struct {
	Connection
	enabled bool
}

func (c heightConnection) SupportsCursorHeight() bool     { return c.enabled }
func (c heightConnection) SupportsInputPreparation() bool { return c.enabled }

func TestConnectionEvidencePreservesHeightNegotiation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		c := newSessionConnectionEvidence(heightConnection{enabled: enabled}, nil)
		if c.SupportsCursorHeight() != enabled {
			t.Fatal("evidence wrapper lost ALPN capability")
		}
	}
}

type heightBoundaryHook struct{ height control.EdgeHeight }

func (h heightBoundaryHook) RunWithBoundary(ctx context.Context, _ *wininput.SourceSession, boundary wininput.BoundaryObserver, ready func()) error {
	ready()
	if err := wininput.ObserveBoundaryHeight(boundary, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 99, h.height); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestSupervisedSessionTransmitsNegotiatedHeight(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, height := range []control.EdgeHeight{1, 65536} {
			func() {
				left, right := openConnectionPair(t)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				done := make(chan error, 1)
				defer func() { cancel(); _ = right.Close(); <-done }()
				connection := &supervisedConnection{Connection: heightConnection{Connection: left, enabled: enabled}}
				go func() {
					_, err := RunSession(ctx, DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10}), Dependencies{
						Connection: connection, Hook: heightBoundaryHook{height: height}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{},
					})
					done <- err
				}()
				_ = right.SetReadDeadline(time.Now().Add(5 * time.Second))
				var command transport.ControlCommand
				// Negotiated height also enables the real preparation wrapper.
				// One geometry-free hint may race this single boundary request.
				for range 2 {
					frame, err := right.ReadFrame()
					if err != nil {
						t.Fatal(err)
					}
					command, err = transport.DecodeControlPayload(frame.Payload)
					if err != nil {
						t.Fatal(err)
					}
					if command.Kind != transport.ControlPrepareInput {
						break
					}
				}
				want := height
				if !enabled {
					want = 0
				}
				if command.Kind != transport.ControlLeaseRequest || command.Height != want {
					t.Fatalf("supervised session lost height: enabled=%v got=%d want=%d", enabled, command.Height, want)
				}
			}()
		}
	}
}
