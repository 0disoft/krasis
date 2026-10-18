//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/lab"
	"github.com/0disoft/krasis/internal/winmessage"
)

const (
	wmPointerShadowReady = 0x8003
)

type pointerShadowCounters struct {
	edge       string
	left       int32
	right      int32
	legacy     legacyPointerDeltaTracker
	hookAtEdge atomic.Bool

	rawRelativePackets        atomic.Uint64
	rawAbsolutePackets        atomic.Uint64
	rawZeroMovementPackets    atomic.Uint64
	rawOutwardPackets         atomic.Uint64
	rawInwardPackets          atomic.Uint64
	rawOutwardWhileHookAtEdge atomic.Uint64
	rawZeroDevicePackets      atomic.Uint64

	hookPhysicalMoves        atomic.Uint64
	hookInjectedMoves        atomic.Uint64
	hookEdgeMoves            atomic.Uint64
	hookUnchangedAtEdgeMoves atomic.Uint64
	hookOutwardAtEdgeMoves   atomic.Uint64
	hookInwardAtEdgeMoves    atomic.Uint64
}

type legacyPointerDeltaTracker struct {
	last point
	has  bool
}

func (tracker *legacyPointerDeltaTracker) observe(current point) (deltaX, deltaY int32, hadPrevious bool) {
	previous := tracker.last
	hadPrevious = tracker.has
	tracker.last = current
	tracker.has = true
	if !hadPrevious {
		return 0, 0, false
	}
	return current.X - previous.X, current.Y - previous.Y, true
}

func newPointerShadowCounters(edge string, left, width int32) (*pointerShadowCounters, error) {
	if edge != "LEFT" && edge != "RIGHT" {
		return nil, errors.New("pointer shadow edge must be LEFT or RIGHT")
	}
	if width <= 1 {
		return nil, errors.New("pointer shadow virtual screen width must be greater than one")
	}
	right := int64(left) + int64(width) - 1
	if right > int64(^uint32(0)>>1) || right < -int64(^uint32(0)>>1)-1 {
		return nil, errors.New("pointer shadow virtual screen coordinates overflow int32")
	}
	return &pointerShadowCounters{edge: edge, left: left, right: int32(right)}, nil
}

func (counters *pointerShadowCounters) observeHook(data mouseHookData) {
	deltaX, _, hadPrevious := counters.legacy.observe(data.Point)
	injected := data.Flags&llmhfInjected != 0 || data.ExtraInfo == krasisInjectedExtraInfo || data.ExtraInfo == selfTestExtraInfo
	if injected {
		counters.hookInjectedMoves.Add(1)
		return
	}
	counters.hookPhysicalMoves.Add(1)
	atEdge := data.Point.X <= counters.left
	if counters.edge == "RIGHT" {
		atEdge = data.Point.X >= counters.right
	}
	counters.hookAtEdge.Store(atEdge)
	if !atEdge {
		return
	}
	counters.hookEdgeMoves.Add(1)
	if !hadPrevious {
		return
	}
	if deltaX == 0 {
		counters.hookUnchangedAtEdgeMoves.Add(1)
	}
	if counters.outward(deltaX) {
		counters.hookOutwardAtEdgeMoves.Add(1)
	}
	if counters.inward(deltaX) {
		counters.hookInwardAtEdgeMoves.Add(1)
	}
}

func (counters *pointerShadowCounters) observeRaw(header rawInputHeader, mouse rawMouse) {
	if header.Device == 0 {
		counters.rawZeroDevicePackets.Add(1)
	}
	if mouse.Flags&mouseMoveAbsolute != 0 {
		counters.rawAbsolutePackets.Add(1)
		return
	}
	counters.rawRelativePackets.Add(1)
	if mouse.LastX == 0 && mouse.LastY == 0 {
		counters.rawZeroMovementPackets.Add(1)
	}
	if counters.outward(mouse.LastX) {
		counters.rawOutwardPackets.Add(1)
		if counters.hookAtEdge.Load() {
			counters.rawOutwardWhileHookAtEdge.Add(1)
		}
	}
	if counters.inward(mouse.LastX) {
		counters.rawInwardPackets.Add(1)
	}
}

func (counters *pointerShadowCounters) outward(deltaX int32) bool {
	if counters.edge == "RIGHT" {
		return deltaX > 0
	}
	return deltaX < 0
}

func (counters *pointerShadowCounters) inward(deltaX int32) bool {
	if counters.edge == "RIGHT" {
		return deltaX < 0
	}
	return deltaX > 0
}

func (counters *pointerShadowCounters) snapshot(elapsed time.Duration) lab.PointerShadowObservation {
	return lab.PointerShadowObservation{
		ElapsedMillis: elapsed.Milliseconds(),
		Raw: lab.PointerShadowRawCounts{
			RelativePackets:        counters.rawRelativePackets.Load(),
			AbsolutePackets:        counters.rawAbsolutePackets.Load(),
			ZeroMovementPackets:    counters.rawZeroMovementPackets.Load(),
			OutwardPackets:         counters.rawOutwardPackets.Load(),
			InwardPackets:          counters.rawInwardPackets.Load(),
			OutwardWhileHookAtEdge: counters.rawOutwardWhileHookAtEdge.Load(),
			ZeroDevicePackets:      counters.rawZeroDevicePackets.Load(),
		},
		Hook: lab.PointerShadowHookCounts{
			PhysicalMoves:        counters.hookPhysicalMoves.Load(),
			InjectedMoves:        counters.hookInjectedMoves.Load(),
			EdgeMoves:            counters.hookEdgeMoves.Load(),
			UnchangedAtEdgeMoves: counters.hookUnchangedAtEdgeMoves.Load(),
			OutwardAtEdgeMoves:   counters.hookOutwardAtEdgeMoves.Load(),
			InwardAtEdgeMoves:    counters.hookInwardAtEdgeMoves.Load(),
		},
	}
}

func (platform *Platform) PointerShadow(ctx context.Context, options lab.PointerShadowOptions) (lab.PointerShadowObservation, error) {
	if platform == nil {
		return lab.PointerShadowObservation{}, errors.New("pointer shadow platform is not initialized")
	}
	if ctx == nil {
		return lab.PointerShadowObservation{}, errors.New("pointer shadow context is required")
	}
	if options.Duration <= 0 {
		return lab.PointerShadowObservation{}, errors.New("pointer shadow duration must be positive")
	}
	if err := ctx.Err(); err != nil {
		return lab.PointerShadowObservation{}, err
	}
	counters, err := newPointerShadowCounters(
		options.Edge,
		systemMetric(smXVIRTUALSCREEN),
		systemMetric(smCXVIRTUALSCREEN),
	)
	if err != nil {
		return lab.PointerShadowObservation{}, err
	}
	releaseHookSession, err := acquireHookSession(ctx)
	if err != nil {
		return lab.PointerShadowObservation{}, err
	}
	defer releaseHookSession()

	type result struct {
		observation lab.PointerShadowObservation
		err         error
	}
	completed := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		observation, runErr := pointerShadowOnDedicatedThread(ctx, options, counters)
		completed <- result{observation: observation, err: runErr}
	}()
	value := <-completed
	return value.observation, value.err
}

func pointerShadowOnDedicatedThread(
	ctx context.Context,
	options lab.PointerShadowOptions,
	counters *pointerShadowCounters,
) (lab.PointerShadowObservation, error) {
	threadID, _, _ := procGetCurrentThreadID.Call()
	var bootstrapMessage message
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrapMessage)), 0, 0, 0, pmNoRemove)

	window, err := createRawInputMessageWindow("KrasisPointerShadow")
	if err != nil {
		return lab.PointerShadowObservation{}, err
	}
	defer procDestroyWindow.Call(window)
	if err := registerRawMouseInput(window); err != nil {
		return lab.PointerShadowObservation{}, err
	}
	defer unregisterRawMouseInput()
	rawReader, err := newRawMouseReader()
	if err != nil {
		return lab.PointerShadowObservation{}, err
	}

	mouseCallback := syscall.NewCallback(func(code int, wParam uintptr, lParam uintptr) uintptr {
		if code == hcAction && uint32(wParam) == wmMouseMove && lParam != 0 {
			counters.observeHook(*(*mouseHookData)(unsafe.Pointer(lParam)))
		}
		return callNextHook(code, wParam, lParam)
	})
	module, _, callErr := procGetModuleHandleW.Call(0)
	if module == 0 {
		return lab.PointerShadowObservation{}, fmt.Errorf("GetModuleHandleW failed: %w", callErr)
	}
	mouseHook, _, callErr := procSetWindowsHookExW.Call(whMouseLL, mouseCallback, module, 0)
	if mouseHook == 0 {
		return lab.PointerShadowObservation{}, fmt.Errorf("SetWindowsHookExW(WH_MOUSE_LL) failed: %w", callErr)
	}
	defer procUnhookWindowsHookEx.Call(mouseHook)

	if options.Ready != nil {
		result, _, postErr := procPostThreadMessageW.Call(threadID, wmPointerShadowReady, 0, 0)
		if result == 0 {
			return lab.PointerShadowObservation{}, fmt.Errorf("PostThreadMessageW(pointer shadow ready) failed: %w", postErr)
		}
	}
	started := time.Now()
	stopMessageLoop := make(chan struct{})
	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		select {
		case <-ctx.Done():
			procPostThreadMessageW.Call(threadID, wmQuit, 0, 0)
		case <-stopMessageLoop:
		}
	}()

	loopErr := runPointerShadowMessageLoop(options.Ready, counters, rawReader)
	close(stopMessageLoop)
	<-stopDone
	observation := counters.snapshot(time.Since(started))
	if loopErr != nil {
		return observation, loopErr
	}
	return observation, ctx.Err()
}

func runPointerShadowMessageLoop(ready func(), counters *pointerShadowCounters, rawReader *rawMouseReader) error {
	var msg message
	visit := func(header rawInputHeader, mouse rawMouse) error {
		counters.observeRaw(header, mouse)
		return nil
	}
	for {
		result, callErr := winmessage.Get((*winmessage.Message)(unsafe.Pointer(&msg)))
		switch int32(result) {
		case -1:
			return fmt.Errorf("GetMessageW(pointer shadow) failed: %w", callErr)
		case 0:
			return nil
		}
		switch msg.Message {
		case wmPointerShadowReady:
			if ready != nil {
				ready()
				ready = nil
			}
			continue
		case wmInput:
			if err := rawReader.readCurrent(msg.LParam, visit); err != nil {
				return err
			}
			if err := rawReader.drainBuffered(visit); err != nil {
				return err
			}
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
