package wininput

import "time"

// HookTimingObserver must only update bounded counters and must never wait or do
// I/O. Timing includes the downstream CallNextHookEx chain; it is not evidence
// that Krasis or another specific hook caused a delay.
type HookTimingObserver interface {
	ObserveHookStarted(time.Time)
	ObserveHookCompleted(time.Time)
	ObserveRawInput(time.Time)
}

func timedSourceCallback(next func(int, uintptr, uintptr) uintptr, observer HookTimingObserver) func(int, uintptr, uintptr) uintptr {
	if observer == nil {
		return next
	}
	return func(code int, w, l uintptr) uintptr {
		observer.ObserveHookStarted(time.Now())
		defer func() { observer.ObserveHookCompleted(time.Now()) }()
		return next(code, w, l)
	}
}
