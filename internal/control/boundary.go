package control

import (
	"errors"
	"fmt"
)

type ScreenEdge string

const (
	ScreenEdgeLeft  ScreenEdge = "LEFT"
	ScreenEdgeRight ScreenEdge = "RIGHT"
)

var ErrInvalidBoundaryConfig = errors.New("invalid screen boundary configuration")

type BoundaryConfig struct {
	Edge          ScreenEdge
	ScreenLeft    int32
	ScreenWidth   int32
	ActivationGap int32
	RearmDistance int32
}

type BoundaryIntent struct {
	Edge ScreenEdge
	X    int32
}

type BoundaryDetector struct {
	config  BoundaryConfig
	right   int32
	armed   bool
	pressed PressedState
}

func NewBoundaryDetector(config BoundaryConfig) (*BoundaryDetector, error) {
	if config.Edge != ScreenEdgeLeft && config.Edge != ScreenEdgeRight {
		return nil, fmt.Errorf("%w: edge must be LEFT or RIGHT", ErrInvalidBoundaryConfig)
	}
	if config.ScreenWidth <= 1 {
		return nil, fmt.Errorf("%w: screen width must be greater than one", ErrInvalidBoundaryConfig)
	}
	if config.ActivationGap < 0 || config.ActivationGap >= config.ScreenWidth {
		return nil, fmt.Errorf("%w: activation gap is outside the screen", ErrInvalidBoundaryConfig)
	}
	if config.RearmDistance <= config.ActivationGap || config.RearmDistance >= config.ScreenWidth {
		return nil, fmt.Errorf("%w: rearm distance must be greater than activation gap and inside the screen", ErrInvalidBoundaryConfig)
	}
	right := int64(config.ScreenLeft) + int64(config.ScreenWidth) - 1
	if right > int64(^uint32(0)>>1) || right < -int64(^uint32(0)>>1)-1 {
		return nil, fmt.Errorf("%w: screen coordinates overflow int32", ErrInvalidBoundaryConfig)
	}
	return &BoundaryDetector{config: config, right: int32(right), armed: true}, nil
}

func (detector *BoundaryDetector) Observe(action InputAction, pointerX int32) (BoundaryIntent, bool, error) {
	if detector == nil {
		return BoundaryIntent{}, false, errors.New("boundary detector is required")
	}
	if err := action.Validate(); err != nil {
		return BoundaryIntent{}, false, err
	}
	if token, pressed, discrete := action.PressedToken(); discrete {
		if pressed {
			if _, err := detector.pressed.RecordPress(token); err != nil {
				return BoundaryIntent{}, false, err
			}
		} else {
			if _, err := detector.pressed.RecordRelease(token); err != nil {
				return BoundaryIntent{}, false, err
			}
		}
		return BoundaryIntent{}, false, nil
	}
	if action.Kind != InputPointerMove {
		return BoundaryIntent{}, false, nil
	}
	detector.rearm(pointerX)
	if !detector.armed || detector.pressed.Count() != 0 || !detector.reached(pointerX, action.DeltaX) {
		return BoundaryIntent{}, false, nil
	}
	detector.armed = false
	return BoundaryIntent{Edge: detector.config.Edge, X: pointerX}, true, nil
}

func (detector *BoundaryDetector) Armed() bool {
	return detector != nil && detector.armed
}

func (detector *BoundaryDetector) PressedCount() int {
	if detector == nil {
		return 0
	}
	return detector.pressed.Count()
}

func (detector *BoundaryDetector) reached(pointerX, deltaX int32) bool {
	left := detector.config.ScreenLeft
	right := detector.right
	switch detector.config.Edge {
	case ScreenEdgeLeft:
		return pointerX <= left+detector.config.ActivationGap && deltaX < 0
	case ScreenEdgeRight:
		return pointerX >= right-detector.config.ActivationGap && deltaX > 0
	default:
		return false
	}
}

func (detector *BoundaryDetector) rearm(pointerX int32) {
	if detector.armed {
		return
	}
	left := detector.config.ScreenLeft
	right := detector.right
	if detector.config.Edge == ScreenEdgeLeft && pointerX >= left+detector.config.RearmDistance {
		detector.armed = true
	}
	if detector.config.Edge == ScreenEdgeRight && pointerX <= right-detector.config.RearmDistance {
		detector.armed = true
	}
}
