//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/lab"
)

const (
	inputMouse               = 0
	mouseEventMove           = 0x0001
	mouseEventMoveNoCoalesce = 0x2000
	selfTestEventCount       = 2
	selfTestDeadline         = 2 * time.Second
	selfTestReportInterval   = 10 * time.Millisecond
)

var (
	procSendInput    = user32.NewProc("SendInput")
	procGetCursorPos = user32.NewProc("GetCursorPos")
)

type mouseInput struct {
	DX        int32
	DY        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type input struct {
	Type  uint32
	Mouse mouseInput
}

type injectionOutcome struct {
	sent            uint32
	pointerRestored bool
	err             error
}

func (p *Platform) SelfTestInject(ctx context.Context) (lab.SelfTestResult, error) {
	testContext, cancel := context.WithTimeout(ctx, selfTestDeadline)
	defer cancel()

	ready := make(chan struct{})
	injectionDone := make(chan injectionOutcome, 1)
	go func() {
		select {
		case <-ready:
			sent, pointerRestored, err := sendTaggedRelativeMoves()
			injectionDone <- injectionOutcome{sent: sent, pointerRestored: pointerRestored, err: err}
			if err != nil {
				cancel()
			}
		case <-testContext.Done():
			injectionDone <- injectionOutcome{err: testContext.Err()}
		}
	}()

	var snapshotMu sync.Mutex
	var latest lab.CaptureSnapshot
	captureErr := p.Capture(testContext, lab.CaptureOptions{
		ReportInterval: selfTestReportInterval,
		Ready: func() {
			close(ready)
		},
	}, func(snapshot lab.CaptureSnapshot) {
		snapshotMu.Lock()
		latest = snapshot
		observed := snapshot.Mouse.SelfTestInjectedMove
		snapshotMu.Unlock()
		if observed >= selfTestEventCount {
			cancel()
		}
	})
	cancel()
	outcome := <-injectionDone

	if captureErr != nil && !errors.Is(captureErr, context.Canceled) && !errors.Is(captureErr, context.DeadlineExceeded) {
		return lab.SelfTestResult{}, captureErr
	}

	snapshotMu.Lock()
	observed := latest.Mouse.SelfTestInjectedMove
	snapshotMu.Unlock()

	return classifySelfTest(outcome, observed), nil
}

func classifySelfTest(outcome injectionOutcome, observed uint64) lab.SelfTestResult {
	result := lab.SelfTestResult{
		SchemaVersion:   lab.SelfTestSchemaVersion,
		Sent:            outcome.sent,
		Observed:        observed,
		PointerRestored: outcome.pointerRestored,
	}
	switch {
	case outcome.err != nil:
		result.FailureCode = "send_input_failed"
	case outcome.sent != selfTestEventCount:
		result.FailureCode = "send_input_incomplete"
	case observed < selfTestEventCount:
		result.FailureCode = "injected_events_not_observed"
	case !outcome.pointerRestored:
		result.FailureCode = "pointer_position_changed"
	default:
		result.Passed = true
	}
	return result
}

func sendTaggedRelativeMoves() (uint32, bool, error) {
	before, err := cursorPosition()
	if err != nil {
		return 0, false, err
	}

	inputs := [selfTestEventCount]input{
		{
			Type: inputMouse,
			Mouse: mouseInput{
				DX:        1,
				Flags:     mouseEventMove | mouseEventMoveNoCoalesce,
				ExtraInfo: selfTestExtraInfo,
			},
		},
		{
			Type: inputMouse,
			Mouse: mouseInput{
				DX:        -1,
				Flags:     mouseEventMove | mouseEventMoveNoCoalesce,
				ExtraInfo: selfTestExtraInfo,
			},
		},
	}

	sent, _, callErr := procSendInput.Call(
		selfTestEventCount,
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(input{}),
	)
	if uint32(sent) != selfTestEventCount {
		return uint32(sent), false, fmt.Errorf("SendInput inserted %d of %d events: %w", sent, selfTestEventCount, callErr)
	}
	after, err := cursorPosition()
	if err != nil {
		return uint32(sent), false, err
	}
	return uint32(sent), before == after, nil
}

func cursorPosition() (point, error) {
	var position point
	result, _, callErr := procGetCursorPos.Call(uintptr(unsafe.Pointer(&position)))
	if result == 0 {
		return point{}, fmt.Errorf("GetCursorPos failed: %w", callErr)
	}
	return position, nil
}
