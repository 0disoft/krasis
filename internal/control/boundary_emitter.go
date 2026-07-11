package control

import "errors"

const BoundaryIntentQueueCapacity = 1

var ErrBoundaryIntentQueueFull = errors.New("boundary intent queue is full")

// BoundaryIntentEmitter owns the single-producer detector and exposes a
// bounded, non-blocking handoff to the session orchestrator. Observe must be
// called from one input-hook thread; Intents may be consumed concurrently.
type BoundaryIntentEmitter struct {
	detector *BoundaryDetector
	intents  chan BoundaryIntent
}

func NewBoundaryIntentEmitter(detector *BoundaryDetector) (*BoundaryIntentEmitter, error) {
	if detector == nil {
		return nil, errors.New("boundary detector is required")
	}
	return &BoundaryIntentEmitter{
		detector: detector,
		intents:  make(chan BoundaryIntent, BoundaryIntentQueueCapacity),
	}, nil
}

func (emitter *BoundaryIntentEmitter) Observe(action InputAction, pointerX int32) error {
	if emitter == nil || emitter.detector == nil || emitter.intents == nil {
		return errors.New("boundary intent emitter is not initialized")
	}
	intent, fired, err := emitter.detector.Observe(action, pointerX)
	if err != nil || !fired {
		return err
	}
	select {
	case emitter.intents <- intent:
		return nil
	default:
		return ErrBoundaryIntentQueueFull
	}
}

func (emitter *BoundaryIntentEmitter) Intents() <-chan BoundaryIntent {
	if emitter == nil {
		return nil
	}
	return emitter.intents
}
