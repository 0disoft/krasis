package agent

import (
	"context"
	"github.com/0disoft/krasis/internal/control"
	"testing"
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

func (c heightConnection) SupportsCursorHeight() bool { return c.enabled }

func TestConnectionEvidencePreservesHeightNegotiation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		c := newSessionConnectionEvidence(heightConnection{enabled: enabled}, nil)
		if c.SupportsCursorHeight() != enabled {
			t.Fatal("evidence wrapper lost ALPN capability")
		}
	}
}
