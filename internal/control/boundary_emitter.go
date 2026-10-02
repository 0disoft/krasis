package control

import (
	"errors"
	"time"
)

const BoundaryIntentQueueCapacity = 1

var ErrBoundaryIntentQueueFull = errors.New("boundary intent queue is full")

// BoundaryIntentEmitter owns the serialized detector and exposes a bounded,
// non-blocking handoff to the session orchestrator. Observe must not be called
// concurrently; Intents may be consumed concurrently.
type BoundaryIntentEmitter struct {
	detector *BoundaryDetector
	intents  chan BoundaryIntent
	observer func(BoundaryIntent)
}

func NewBoundaryIntentEmitter(detector *BoundaryDetector) (*BoundaryIntentEmitter, error) {
	return NewBoundaryIntentEmitterObserved(detector, nil)
}

func NewBoundaryIntentEmitterObserved(detector *BoundaryDetector, observer func(BoundaryIntent)) (*BoundaryIntentEmitter, error) {
	if detector == nil {
		return nil, errors.New("boundary detector is required")
	}
	return &BoundaryIntentEmitter{
		detector: detector,
		intents:  make(chan BoundaryIntent, BoundaryIntentQueueCapacity),
		observer: observer,
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
	// Preserve the local monotonic decision time before the consumer can run.
	intent.DecidedAt = time.Now()
	select {
	case emitter.intents <- intent:
		if emitter.observer != nil {
			emitter.observer(intent)
		}
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
