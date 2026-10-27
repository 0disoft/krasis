//go:build windows

package inputworker

import (
	"context"
	"time"
)

func (c *CaptureSource) SetObserver(emit func(string)) { c.observe = emit }
func (c *CaptureSource) observeCursor(stage string, started time.Time) {
	if c.observe == nil {
		return
	}
	bucket := timingBucket(time.Since(started))
	if bucket != 0 {
		c.observe("installed_return_cursor_capture_" + stage + "_" + []string{"", "lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"}[bucket])
	}
}

// Reuse the authenticated hook-thread shield. Preparation snapshots the cursor;
// show is allowed once and only for that exact snapshot sequence. No fresh
// process, desktop selector, input command or second cursor shield is created.
func (c *CaptureSource) PrepareReturnCursor() func() {
	live := c.client()
	if live == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	started := time.Now()
	response, err := live.call(ctx, sourceCursorPrepare)
	cancel()
	c.observeCursor("prepare", started)
	if err != nil {
		return nil
	}
	if response.status == sourceCursorUnavailable {
		if c.observe != nil {
			c.observe("installed_return_cursor_capture_unavailable")
		}
		return nil
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		started := time.Now()
		_, err := live.call(ctx, sourceCursorShow, response.reply)
		c.observeCursor("show", started)
		if c.observe != nil {
			code := "installed_return_cursor_capture_queued"
			if err != nil {
				code = "installed_return_cursor_capture_failed"
			}
			c.observe(code)
		}
	}
}
func (c *CaptureSource) RestoreReturnCursor() {
	live := c.client()
	if live == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _ = live.call(ctx, sourceCursorRestore)
	c.observeCursor("restore", started)
}
