//go:build windows

package wininput

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procSourceForegroundWait = user32.NewProc("MsgWaitForMultipleObjectsEx")
var callSourceForegroundWait = procSourceForegroundWait.Call

func waitSourceForegroundMessages(event windows.Handle, interval time.Duration) error {
	var count uintptr
	var handles *windows.Handle
	if event != 0 {
		count, handles = 1, &event
	}
	// PeekMessage marks old queued input as seen without removing it. Wake only
	// for new input/system activation, not continuously for that old input. The
	// owner still pumps only nonqueued/internal events; queued commands and input
	// must remain for its outer message loop.
	millis := (interval + time.Millisecond - 1) / time.Millisecond
	result, _, err := callSourceForegroundWait(count, uintptr(unsafe.Pointer(handles)), uintptr(millis), 0x04ff, 0) // QS_ALLINPUT, no MWMO_INPUTAVAILABLE
	if uint32(result) == ^uint32(0) {
		return touchpadWin32Error("MsgWaitForMultipleObjectsEx(source foreground)", err)
	}
	return nil
}

var sourceForegroundExchangeWait = func(event windows.Handle) error {
	// Wake for pending activation, never dispatch queued input,
	// commands or WM_QUIT inside an activation command. The short cap also
	// observes cancellation. The reply event wakes immediately, avoiding a
	// polling delay rounded to the system timer resolution on every handoff.
	return waitSourceForegroundMessages(event, sourceForegroundTransitionInterval)
}

// One exchange per activation, owned by executeProfile. No background request
// can survive a cancelled command and reactivate a retired HWND. The existing
// monitor desktop scope supplies the requester's binding/authority; the window
// owner independently checks authority before showing and committing capture.
type sourceForegroundExchange struct {
	ready     chan *sourceForegroundRequest
	cancelled chan struct{}
	deadline  time.Time
}

type sourceForegroundRequest struct {
	event    windows.Handle
	refs     atomic.Int32
	reply    chan error
	current  *sourceWheelCaptureRun
	deadline time.Time
}

func (request *sourceForegroundRequest) release() {
	if request.refs.Add(-1) == 0 {
		windows.CloseHandle(request.event)
	}
}

func (request *sourceForegroundRequest) complete(current *sourceWheelCaptureRun, reply error) {
	defer request.release()
	request.reply <- reply
	if err := windows.SetEvent(request.event); err != nil {
		current.fail(err)
	}
}

func newSourceForegroundExchange() *sourceForegroundExchange {
	return &sourceForegroundExchange{
		ready:     make(chan *sourceForegroundRequest),
		cancelled: make(chan struct{}),
		deadline:  time.Now().Add(sourceWheelCaptureCallTimeout),
	}
}

func (exchange *sourceForegroundExchange) request(current *sourceWheelCaptureRun) error {
	if err := current.err(); err != nil {
		return err
	}
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		current.setError(err)
		return err
	}
	request := &sourceForegroundRequest{event: event, reply: make(chan error, 1), current: current, deadline: exchange.deadline}
	request.refs.Store(2) // Window owner and the one accepted command caller.
	defer request.release()
	select {
	case exchange.ready <- request:
	case <-exchange.cancelled:
		request.release() // No caller accepted the second reference.
		return ErrSourceCaptureCommandTimeout
	}
	for {
		select {
		case reply := <-request.reply:
			return exchange.finish(current, reply)
		case <-exchange.cancelled:
			return ErrSourceCaptureCommandTimeout
		default:
		}
		if err := current.err(); err != nil {
			return err
		}
		// Even a cross-queue request can require the target to service sent
		// activation messages. Parking this owner on a Go channel makes Windows
		// wait for its responsiveness timeout before the caller can reply.
		sourceForegroundTransitionDispatch(current.window)
		if err := current.err(); err != nil {
			return err
		}
		// The native requester may complete while PeekMessage services internal
		// activation. Consume its reply before entering another native wait.
		select {
		case reply := <-request.reply:
			return exchange.finish(current, reply)
		case <-exchange.cancelled:
			return ErrSourceCaptureCommandTimeout
		default:
		}
		if err := sourceForegroundExchangeWait(request.event); err != nil {
			current.setError(err)
			return err
		}
	}
}

func (exchange *sourceForegroundExchange) finish(current *sourceWheelCaptureRun, reply error) error {
	if reply != nil {
		return reply
	}
	select {
	case <-exchange.cancelled:
		return ErrSourceCaptureCommandTimeout
	default:
	}
	if !time.Now().Before(exchange.deadline) {
		current.fail(ErrSourceCaptureCommandTimeout)
		return ErrSourceCaptureCommandTimeout
	}
	if err := current.err(); err != nil {
		return err
	}
	// The reply proves only native completion. A fresh foreground observation
	// can prove that no activation nudge remains to service. PeekMessage can run
	// sent callbacks even with PM_NOREMOVE; do not repeat it for an owned window.
	foreground := sourceWheelCaptureGetForeground()
	if err := current.err(); err != nil {
		return err
	}
	if foreground != current.window || current.window == 0 {
		sourceForegroundTransitionDispatch(current.window)
	}
	// activateWindowWithForeground still independently verifies ownership before
	// committing capture; an intervening focus change retains the fallback.
	return current.err()
}

func (current *sourceWheelCaptureRun) requestForegroundOnMonitorThread(ctx context.Context, deadline time.Time) error {
	if err := ctx.Err(); err != nil {
		current.setError(err)
		return err
	}
	if err := current.err(); err != nil {
		return err
	}
	// A ready request and the timer can both be selectable. Never start native
	// activation after the same absolute budget merely because ready won.
	if !time.Now().Before(deadline) {
		current.fail(ErrSourceCaptureCommandTimeout)
		return ErrSourceCaptureCommandTimeout
	}
	if err := sourceLoginDesktopCheck(ctx); err != nil {
		current.setError(err)
		return err
	}
	// The fresh desktop query can yield; fence its completion before Win32.
	if err := ctx.Err(); err != nil {
		current.setError(err)
		return err
	}
	if err := current.err(); err != nil {
		return err
	}
	if !time.Now().Before(deadline) {
		current.fail(ErrSourceCaptureCommandTimeout)
		return ErrSourceCaptureCommandTimeout
	}
	err := sourceWheelCaptureSetForeground(current.window)
	return errors.Join(err, current.err())
}
