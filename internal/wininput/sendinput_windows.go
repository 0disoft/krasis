//go:build windows

package wininput

import (
	"context"
	"errors"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	keyEventExtended = 0x0001
	keyEventKeyUp    = 0x0002
	keyEventScanCode = 0x0008

	mouseEventMove           = 0x0001
	mouseEventLeftDown       = 0x0002
	mouseEventLeftUp         = 0x0004
	mouseEventRightDown      = 0x0008
	mouseEventRightUp        = 0x0010
	mouseEventMiddleDown     = 0x0020
	mouseEventMiddleUp       = 0x0040
	mouseEventWheel          = 0x0800
	mouseEventXDown          = 0x0080
	mouseEventXUp            = 0x0100
	mouseEventHWheel         = 0x1000
	mouseEventMoveNoCoalesce = 0x2000

	xButton1 = 1
	xButton2 = 2

	krasisInjectedExtraInfo = 0x4B524153
)

var procSendInput = user32.NewProc("SendInput")

var callSendInput = func(count uintptr, inputs *input, size uintptr) (uintptr, uintptr, error) {
	return procSendInput.Call(count, uintptr(unsafe.Pointer(inputs)), size)
}

type mouseInput struct {
	DX        int32
	DY        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type keyboardInput struct {
	VirtualKey uint16
	ScanCode   uint16
	Flags      uint32
	Time       uint32
	ExtraInfo  uintptr
}

type input struct {
	Type  uint32
	Mouse mouseInput
}

func keyboardEvent(scanCode uint16, extended, released bool) input {
	flags := uint32(keyEventScanCode)
	// Also tolerate the old sender's E0-marked right Shift on injection.
	// Do not clear E0 for Ctrl, Alt, or Print Screen's synthetic shift codes.
	if extended && uint32(scanCode) != scanCodeRightShift {
		flags |= keyEventExtended
	}
	if released {
		flags |= keyEventKeyUp
	}
	event := input{Type: inputKeyboard}
	*(*keyboardInput)(unsafe.Pointer(&event.Mouse)) = keyboardInput{
		ScanCode:  scanCode,
		Flags:     flags,
		ExtraInfo: krasisInjectedExtraInfo,
	}
	return event
}

func mouseEvent(flags uint32, data uint32, deltaX, deltaY int32) input {
	return input{
		Type: inputMouse,
		Mouse: mouseInput{
			DX:        deltaX,
			DY:        deltaY,
			MouseData: data,
			Flags:     flags,
			ExtraInfo: krasisInjectedExtraInfo,
		},
	}
}

func sendInputs(ctx context.Context, events []input) error {
	if ctx == nil {
		return injectionFailure(application.InjectionNotApplied, len(events), 0, errors.New("context is required"))
	}
	if err := ctx.Err(); err != nil {
		return injectionFailure(application.InjectionNotApplied, len(events), 0, err)
	}
	if len(events) == 0 {
		return injectionFailure(application.InjectionNotApplied, 0, 0, errors.New("at least one event is required"))
	}
	probe := handoff.WorkerApplyProbe(ctx)
	var started time.Time
	if probe != nil {
		started = time.Now()
	}
	sent, _, callErr := callSendInput(
		uintptr(len(events)),
		&events[0],
		unsafe.Sizeof(input{}),
	)
	if probe != nil {
		probe.RecordSubmission(time.Since(started))
	}
	inserted := int(sent)
	if inserted == len(events) {
		return nil
	}
	certainty := application.InjectionOutcomeUnknown
	if inserted == 0 {
		certainty = application.InjectionNotApplied
	} else if inserted > 0 && inserted < len(events) {
		certainty = application.InjectionPartiallyApplied
	}
	return injectionFailure(certainty, len(events), inserted, callErr)
}

func injectionFailure(certainty application.InjectionCertainty, requested, inserted int, cause error) error {
	if cause == nil {
		cause = errors.New("SendInput did not insert the requested event batch")
	}
	return &application.InjectionError{
		Certainty: certainty,
		Requested: requested,
		Inserted:  inserted,
		Cause:     errors.Join(ErrInputInjection, cause),
	}
}
