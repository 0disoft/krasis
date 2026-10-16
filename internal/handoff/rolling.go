package handoff

import "time"

// NewRolling keeps later user-triggered handoffs observable without a polling
// loop or an unbounded history. Each window retains at most Limit attempts.
// Native diagnostic probe counters remain bounded for the collector lifetime.
func NewRolling(emit func(Summary)) *Collector {
	return &Collector{emit: emit, rolling: true}
}

// Caller holds mu. Freeze every old attempt before reusing any slot; callbacks
// carrying old contexts can never append to or retire a later window's attempt.
func (c *Collector) advanceWindowLocked(now time.Time) []func() {
	if !c.rolling || c.started.IsZero() || now.Sub(c.started) < Window {
		return nil
	}
	flushed := c.freezeWindowLocked()
	if c.timer != nil {
		c.timer.Stop()
	}
	c.started = time.Time{}
	c.count = 0
	c.stopped.Store(false)
	return flushed
}

func (c *Collector) freezeWindowLocked() []func() {
	var flushed []func()
	for _, a := range c.active {
		if a != nil && !a.finished {
			flushed = append(flushed, c.retireLocked(a, "incomplete"))
		}
	}
	return flushed
}

func emitProfiles(flushed []func()) {
	for _, emit := range flushed {
		emit()
	}
}

func (c *Collector) expireWindow(window time.Time) {
	c.mu.Lock()
	// Stop does not join an already running timer callback. A callback that
	// waited for mu must not expire the replacement window.
	if !window.IsZero() && !window.Equal(c.started) {
		c.mu.Unlock()
		return
	}
	c.stopped.Store(true)
	flushed := c.freezeWindowLocked()
	c.mu.Unlock()
	emitProfiles(flushed)
}
