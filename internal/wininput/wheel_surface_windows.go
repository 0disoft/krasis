//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/lab"
)

const (
	wheelSurfaceSentinelExtraInfo      = 0x4B525357
	wheelSurfaceSentinelEventsPerPulse = 2
)

type wheelSurfaceCounters struct {
	hookPhysicalMoves               atomic.Uint64
	hookPhysicalButtonDowns         atomic.Uint64
	hookPhysicalButtonUps           atomic.Uint64
	hookPhysicalVerticalWheels      atomic.Uint64
	hookPhysicalHorizontalWheels    atomic.Uint64
	hookSynthesizedVerticalWheels   atomic.Uint64
	hookSynthesizedHorizontalWheels atomic.Uint64

	rawPackets                  atomic.Uint64
	rawZeroDevicePackets        atomic.Uint64
	rawTaggedInjectedPackets    atomic.Uint64
	rawVerticalWheelPackets     atomic.Uint64
	rawHorizontalWheelPackets   atomic.Uint64
	rawVerticalDeltaTotal       atomic.Int64
	rawHorizontalDeltaTotal     atomic.Int64
	rawVerticalDeltaMagnitude   atomic.Uint64
	rawHorizontalDeltaMagnitude atomic.Uint64

	sentinelRequested atomic.Uint64
	sentinelInserted  atomic.Uint64
	sentinelObserved  atomic.Uint64
	sentinelSwallowed atomic.Uint64
}

func (counters *wheelSurfaceCounters) observeHook(message uint32, data mouseHookData) bool {
	if data.ExtraInfo == wheelSurfaceSentinelExtraInfo {
		if message == wmMouseMove {
			counters.sentinelObserved.Add(1)
			counters.sentinelSwallowed.Add(1)
			return true
		}
		return false
	}

	injected := data.Flags&llmhfInjected != 0 || isTaggedMouseInput(data)
	switch message {
	case wmMouseMove:
		if !injected {
			counters.hookPhysicalMoves.Add(1)
		}
	case wmLButtonDown, wmRButtonDown, wmMButtonDown, wmXButtonDown:
		if !injected {
			counters.hookPhysicalButtonDowns.Add(1)
		}
	case wmLButtonUp, wmRButtonUp, wmMButtonUp, wmXButtonUp:
		if !injected {
			counters.hookPhysicalButtonUps.Add(1)
		}
	case wmMouseWheel:
		if injected {
			counters.hookSynthesizedVerticalWheels.Add(1)
		} else {
			counters.hookPhysicalVerticalWheels.Add(1)
		}
	case wmMouseHWheel:
		if injected {
			counters.hookSynthesizedHorizontalWheels.Add(1)
		} else {
			counters.hookPhysicalHorizontalWheels.Add(1)
		}
	}
	return false
}

func (counters *wheelSurfaceCounters) observeRaw(header rawInputHeader, mouse rawMouse) {
	counters.rawPackets.Add(1)
	if header.Device == 0 {
		counters.rawZeroDevicePackets.Add(1)
	}
	if uintptr(mouse.ExtraInformation) == wheelSurfaceSentinelExtraInfo ||
		uintptr(mouse.ExtraInformation) == krasisInjectedExtraInfo ||
		uintptr(mouse.ExtraInformation) == selfTestExtraInfo {
		counters.rawTaggedInjectedPackets.Add(1)
		return
	}
	buttonFlags := uint16(mouse.Buttons)
	delta := int64(int16(uint16(mouse.Buttons >> 16)))
	if buttonFlags&rawMouseWheel != 0 {
		counters.rawVerticalWheelPackets.Add(1)
		counters.rawVerticalDeltaTotal.Add(delta)
		counters.rawVerticalDeltaMagnitude.Add(wheelSurfaceDeltaMagnitude(delta))
	}
	if buttonFlags&rawMouseHWheel != 0 {
		counters.rawHorizontalWheelPackets.Add(1)
		counters.rawHorizontalDeltaTotal.Add(delta)
		counters.rawHorizontalDeltaMagnitude.Add(wheelSurfaceDeltaMagnitude(delta))
	}
}

func wheelSurfaceDeltaMagnitude(delta int64) uint64 {
	if delta < 0 {
		return uint64(-delta)
	}
	return uint64(delta)
}

func (counters *wheelSurfaceCounters) recordSentinelSend(requested, inserted uint64) {
	counters.sentinelRequested.Add(requested)
	counters.sentinelInserted.Add(inserted)
}

func (counters *wheelSurfaceCounters) snapshot(elapsed time.Duration) lab.WheelSurfaceObservation {
	return lab.WheelSurfaceObservation{
		ElapsedMillis: elapsed.Milliseconds(),
		Hook: lab.WheelSurfaceHookCounts{
			PhysicalMoves:               counters.hookPhysicalMoves.Load(),
			PhysicalButtonDowns:         counters.hookPhysicalButtonDowns.Load(),
			PhysicalButtonUps:           counters.hookPhysicalButtonUps.Load(),
			PhysicalVerticalWheels:      counters.hookPhysicalVerticalWheels.Load(),
			PhysicalHorizontalWheels:    counters.hookPhysicalHorizontalWheels.Load(),
			SynthesizedVerticalWheels:   counters.hookSynthesizedVerticalWheels.Load(),
			SynthesizedHorizontalWheels: counters.hookSynthesizedHorizontalWheels.Load(),
		},
		Raw: lab.WheelSurfaceRawCounts{
			Packets:                  counters.rawPackets.Load(),
			ZeroDevicePackets:        counters.rawZeroDevicePackets.Load(),
			TaggedInjectedPackets:    counters.rawTaggedInjectedPackets.Load(),
			VerticalWheelPackets:     counters.rawVerticalWheelPackets.Load(),
			HorizontalWheelPackets:   counters.rawHorizontalWheelPackets.Load(),
			VerticalDeltaTotal:       counters.rawVerticalDeltaTotal.Load(),
			HorizontalDeltaTotal:     counters.rawHorizontalDeltaTotal.Load(),
			VerticalDeltaMagnitude:   counters.rawVerticalDeltaMagnitude.Load(),
			HorizontalDeltaMagnitude: counters.rawHorizontalDeltaMagnitude.Load(),
		},
		Sentinel: lab.WheelSurfaceSentinelCounts{
			Requested: counters.sentinelRequested.Load(),
			Inserted:  counters.sentinelInserted.Load(),
			Observed:  counters.sentinelObserved.Load(),
			Swallowed: counters.sentinelSwallowed.Load(),
		},
	}
}

func (platform *Platform) WheelSurface(ctx context.Context, options lab.WheelSurfaceOptions) (lab.WheelSurfaceObservation, error) {
	if platform == nil {
		return lab.WheelSurfaceObservation{}, errors.New("wheel surface platform is not initialized")
	}
	if ctx == nil {
		return lab.WheelSurfaceObservation{}, errors.New("wheel surface context is required")
	}
	if options.Duration <= 0 {
		return lab.WheelSurfaceObservation{}, errors.New("wheel surface duration must be positive")
	}
	if err := ctx.Err(); err != nil {
		return lab.WheelSurfaceObservation{}, err
	}
	releaseHookSession, err := acquireHookSession(ctx)
	if err != nil {
		return lab.WheelSurfaceObservation{}, err
	}
	defer releaseHookSession()

	counters := &wheelSurfaceCounters{}
	threadContext, cancelThreads := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelThreads()
	readySignals := make(chan struct{}, 2)
	results := make(chan sourceInputThreadResult, 2)
	startSourceInputThread(results, "low-level-hook", func() error {
		return runWheelSurfaceHookThread(threadContext, counters, func() {
			readySignals <- struct{}{}
		})
	})
	startSourceInputThread(results, "raw-mouse", func() error {
		return runWheelSurfaceRawThread(threadContext, counters, func() {
			readySignals <- struct{}{}
		})
	})

	started := time.Now()
	readyCount := 0
	for readyCount < 2 {
		select {
		case <-readySignals:
			readyCount++
		case first := <-results:
			cancelThreads()
			second := <-results
			return counters.snapshot(time.Since(started)), wheelSurfaceThreadError(first, second)
		case <-ctx.Done():
			cancelThreads()
			first := <-results
			second := <-results
			return counters.snapshot(time.Since(started)), errors.Join(ctx.Err(), wheelSurfaceThreadError(first, second))
		}
	}
	if options.Ready != nil {
		options.Ready()
	}
	started = time.Now()
	if err := sendAndRecordWheelSurfaceSentinel(counters); err != nil {
		cancelThreads()
		first := <-results
		second := <-results
		return counters.snapshot(time.Since(started)), errors.Join(err, wheelSurfaceThreadError(first, second))
	}

	timer := time.NewTimer(options.Duration)
	defer timer.Stop()
	var terminalErr error
	var earlyResult *sourceInputThreadResult
	select {
	case <-timer.C:
	case <-ctx.Done():
		terminalErr = ctx.Err()
	case result := <-results:
		earlyResult = &result
	}
	if earlyResult == nil {
		terminalErr = errors.Join(terminalErr, sendAndRecordWheelSurfaceSentinel(counters))
	}

	cancelThreads()
	threadResults := make([]sourceInputThreadResult, 0, 2)
	if earlyResult != nil {
		threadResults = append(threadResults, *earlyResult)
	}
	for len(threadResults) < 2 {
		threadResults = append(threadResults, <-results)
	}
	return counters.snapshot(time.Since(started)), errors.Join(terminalErr, wheelSurfaceThreadError(threadResults...))
}

func runWheelSurfaceHookThread(ctx context.Context, counters *wheelSurfaceCounters, ready func()) error {
	threadID, _, _ := procGetCurrentThreadID.Call()
	var bootstrapMessage message
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrapMessage)), 0, 0, 0, pmNoRemove)

	mouseCallback := syscall.NewCallback(func(code int, wParam uintptr, lParam uintptr) uintptr {
		if code == hcAction && lParam != 0 {
			data := *(*mouseHookData)(unsafe.Pointer(lParam))
			if counters.observeHook(uint32(wParam), data) {
				return 1
			}
		}
		return callNextHook(code, wParam, lParam)
	})
	module, _, callErr := procGetModuleHandleW.Call(0)
	if module == 0 {
		return fmt.Errorf("GetModuleHandleW failed: %w", callErr)
	}
	mouseHook, _, callErr := procSetWindowsHookExW.Call(whMouseLL, mouseCallback, module, 0)
	if mouseHook == 0 {
		return fmt.Errorf("SetWindowsHookExW(WH_MOUSE_LL) failed: %w", callErr)
	}
	cleanupHook := func() error {
		if mouseHook == 0 {
			return nil
		}
		result, _, unhookErr := procUnhookWindowsHookEx.Call(mouseHook)
		mouseHook = 0
		if result == 0 {
			return fmt.Errorf("UnhookWindowsHookEx(WH_MOUSE_LL) failed: %w", unhookErr)
		}
		return nil
	}
	defer cleanupHook()
	if err := postSourceReady(threadID); err != nil {
		return err
	}
	loopErr := runSourceHookMessageLoop(ctx, threadID, ready)
	unhookErr := cleanupHook()
	if loopErr != nil && !errors.Is(loopErr, context.Canceled) {
		return errors.Join(loopErr, unhookErr)
	}
	if unhookErr != nil {
		return unhookErr
	}
	return ctx.Err()
}

func runWheelSurfaceRawThread(ctx context.Context, counters *wheelSurfaceCounters, ready func()) error {
	threadID, _, _ := procGetCurrentThreadID.Call()
	var bootstrapMessage message
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrapMessage)), 0, 0, 0, pmNoRemove)
	window, err := createRawInputMessageWindow("KrasisWheelSurface")
	if err != nil {
		return err
	}
	defer procDestroyWindow.Call(window)
	if err := registerRawMouseInput(window); err != nil {
		return err
	}
	rawRegistered := true
	cleanupRawInput := func() error {
		if !rawRegistered {
			return nil
		}
		rawRegistered = false
		return unregisterRawMouseInput()
	}
	defer cleanupRawInput()
	rawReader, err := newRawMouseReader()
	if err != nil {
		return err
	}
	if err := postSourceReady(threadID); err != nil {
		return err
	}
	visit := func(header rawInputHeader, mouse rawMouse) error {
		counters.observeRaw(header, mouse)
		return nil
	}
	loopErr := runSourceRawMessageLoop(ctx, threadID, ready, rawReader, visit)
	rawInputErr := cleanupRawInput()
	if loopErr != nil && !errors.Is(loopErr, context.Canceled) {
		return errors.Join(loopErr, rawInputErr)
	}
	if rawInputErr != nil {
		return rawInputErr
	}
	return ctx.Err()
}

func sendAndRecordWheelSurfaceSentinel(counters *wheelSurfaceCounters) error {
	requested, inserted, err := sendWheelSurfaceSentinelPulse()
	counters.recordSentinelSend(requested, inserted)
	return err
}

func sendWheelSurfaceSentinelPulse() (uint64, uint64, error) {
	events := [wheelSurfaceSentinelEventsPerPulse]input{
		{
			Type: inputMouse,
			Mouse: mouseInput{
				DX:        1,
				Flags:     mouseEventMove | mouseEventMoveNoCoalesce,
				ExtraInfo: wheelSurfaceSentinelExtraInfo,
			},
		},
		{
			Type: inputMouse,
			Mouse: mouseInput{
				DX:        -1,
				Flags:     mouseEventMove | mouseEventMoveNoCoalesce,
				ExtraInfo: wheelSurfaceSentinelExtraInfo,
			},
		},
	}
	requested := uint64(len(events))
	sent, _, callErr := callSendInput(
		uintptr(len(events)),
		&events[0],
		unsafe.Sizeof(input{}),
	)
	inserted := uint64(sent)
	if inserted == requested {
		return requested, inserted, nil
	}
	cause := callErr
	if errno, ok := cause.(syscall.Errno); ok && errno == 0 {
		cause = nil
	}
	if cause == nil {
		cause = errors.New("SendInput did not insert the complete pointer health sentinel")
	}
	return requested, inserted, fmt.Errorf("pointer health sentinel inserted %d of %d events: %w", inserted, requested, cause)
}

func wheelSurfaceThreadError(results ...sourceInputThreadResult) error {
	joined := make([]error, 0, len(results))
	for _, result := range results {
		if errors.Is(result.err, context.Canceled) {
			continue
		}
		if result.err == nil {
			joined = append(joined, fmt.Errorf("wheel surface %s stopped unexpectedly", result.component))
			continue
		}
		joined = append(joined, fmt.Errorf("wheel surface %s: %w", result.component, result.err))
	}
	return errors.Join(joined...)
}
