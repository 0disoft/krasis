package control

import (
	"errors"
	"testing"
)

func TestBoundaryDetectorFiresOnceAtConfiguredEdge(t *testing.T) {
	detector := newRightBoundaryDetector(t)
	intent, fired, err := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: 4}, 99)
	if err != nil || !fired || intent.Edge != ScreenEdgeRight || intent.X != 99 {
		t.Fatalf("first edge observation intent=%+v fired=%t error=%v", intent, fired, err)
	}
	if _, fired, err := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: 1}, 99); err != nil || fired {
		t.Fatalf("disarmed edge fired=%t error=%v", fired, err)
	}
	if detector.Armed() {
		t.Fatal("detector rearmed without moving inward")
	}
}

func TestBoundaryDetectorRequiresMovementTowardEdge(t *testing.T) {
	detector := newRightBoundaryDetector(t)
	if _, fired, err := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: -1}, 99); err != nil || fired {
		t.Fatalf("inward move fired=%t error=%v", fired, err)
	}
	if _, fired, err := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: 1}, 90); err != nil || fired {
		t.Fatalf("inside move fired=%t error=%v", fired, err)
	}
}

func TestBoundaryDetectorBlocksTransitionWhileInputIsPressed(t *testing.T) {
	detector := newRightBoundaryDetector(t)
	key := InputAction{Kind: InputKeyDown, Code: 0x1E}
	if _, fired, err := detector.Observe(key, 50); err != nil || fired {
		t.Fatalf("key down fired=%t error=%v", fired, err)
	}
	if _, fired, err := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: 4}, 99); err != nil || fired {
		t.Fatalf("pressed edge fired=%t error=%v", fired, err)
	}
	if detector.PressedCount() != 1 {
		t.Fatalf("pressed count = %d", detector.PressedCount())
	}
	if _, _, err := detector.Observe(InputAction{Kind: InputKeyUp, Code: 0x1E}, 99); err != nil {
		t.Fatalf("key up error = %v", err)
	}
	if _, fired, err := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: 1}, 99); err != nil || !fired {
		t.Fatalf("released edge fired=%t error=%v", fired, err)
	}
}

func TestBoundaryDetectorRearmsOnlyAfterConfiguredDistance(t *testing.T) {
	detector := newRightBoundaryDetector(t)
	if _, fired, _ := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: 1}, 99); !fired {
		t.Fatal("initial edge did not fire")
	}
	if _, fired, _ := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: -2}, 95); fired || detector.Armed() {
		t.Fatal("detector rearmed inside hysteresis band")
	}
	if _, fired, _ := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: -10}, 89); fired || !detector.Armed() {
		t.Fatal("detector did not rearm after inward movement")
	}
	if _, fired, _ := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: 10}, 99); !fired {
		t.Fatal("rearmed detector did not fire")
	}
}

func TestLeftBoundarySupportsNegativeVirtualScreenCoordinates(t *testing.T) {
	detector, err := NewBoundaryDetector(BoundaryConfig{
		Edge:          ScreenEdgeLeft,
		ScreenLeft:    -1920,
		ScreenWidth:   1920,
		ActivationGap: 0,
		RearmDistance: 10,
	})
	if err != nil {
		t.Fatalf("NewBoundaryDetector() error = %v", err)
	}
	intent, fired, err := detector.Observe(InputAction{Kind: InputPointerMove, DeltaX: -3}, -1920)
	if err != nil || !fired || intent.X != -1920 {
		t.Fatalf("left intent=%+v fired=%t error=%v", intent, fired, err)
	}
}

func TestBoundaryDetectorRejectsInvalidConfigurationAndAction(t *testing.T) {
	_, err := NewBoundaryDetector(BoundaryConfig{Edge: ScreenEdgeRight, ScreenWidth: 1, RearmDistance: 1})
	if !errors.Is(err, ErrInvalidBoundaryConfig) {
		t.Fatalf("NewBoundaryDetector(invalid) error = %v", err)
	}
	_, err = NewBoundaryDetector(BoundaryConfig{
		Edge:          ScreenEdgeRight,
		ScreenLeft:    2147483640,
		ScreenWidth:   100,
		RearmDistance: 10,
	})
	if !errors.Is(err, ErrInvalidBoundaryConfig) {
		t.Fatalf("NewBoundaryDetector(overflow) error = %v", err)
	}
	detector := newRightBoundaryDetector(t)
	_, _, err = detector.Observe(InputAction{Kind: InputPointerMove}, 99)
	if !errors.Is(err, ErrInvalidInputAction) {
		t.Fatalf("Observe(invalid) error = %v", err)
	}
}

func newRightBoundaryDetector(t *testing.T) *BoundaryDetector {
	t.Helper()
	detector, err := NewBoundaryDetector(BoundaryConfig{
		Edge:          ScreenEdgeRight,
		ScreenLeft:    0,
		ScreenWidth:   100,
		ActivationGap: 0,
		RearmDistance: 10,
	})
	if err != nil {
		t.Fatalf("NewBoundaryDetector() error = %v", err)
	}
	return detector
}
