//go:build windows

package wininput

import (
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceMapperMapsPhysicalScanCodes(t *testing.T) {
	mapper := &sourceMapper{}
	action, emitted, err := mapper.keyboard(wmSysKeyDown, keyboardHookData{ScanCode: 0x38, Flags: llkhfExtended})
	if err != nil || !emitted {
		t.Fatalf("keyboard() emitted=%t error=%v", emitted, err)
	}
	wantCode, _ := EncodeScanCode(0x38, true)
	if action.Kind != control.InputKeyDown || action.Code != wantCode {
		t.Fatalf("action = %+v", action)
	}
	if _, emitted, err := mapper.keyboard(wmKeyDown, keyboardHookData{ScanCode: 0x1E, Flags: llkhfInjected}); err != nil || emitted {
		t.Fatalf("injected keyboard emitted=%t error=%v", emitted, err)
	}
}

func TestSourceMapperRejectsInvalidPhysicalScanCode(t *testing.T) {
	mapper := &sourceMapper{}
	_, emitted, err := mapper.keyboard(wmKeyDown, keyboardHookData{})
	if emitted || !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("keyboard(zero) emitted=%t error=%v", emitted, err)
	}
}

func TestSourceMapperTracksPhysicalPointerDeltasAndRebasesInjectedMoves(t *testing.T) {
	mapper := &sourceMapper{}
	if _, emitted, err := mapper.mouse(wmMouseMove, mouseHookData{Point: point{X: 10, Y: 20}}); err != nil || emitted {
		t.Fatalf("baseline emitted=%t error=%v", emitted, err)
	}
	action, emitted, err := mapper.mouse(wmMouseMove, mouseHookData{Point: point{X: 13, Y: 18}})
	if err != nil || !emitted || action.DeltaX != 3 || action.DeltaY != -2 {
		t.Fatalf("physical move action=%+v emitted=%t error=%v", action, emitted, err)
	}
	if _, emitted, err := mapper.mouse(wmMouseMove, mouseHookData{Point: point{X: 100, Y: 100}, Flags: llmhfInjected}); err != nil || emitted {
		t.Fatalf("injected move emitted=%t error=%v", emitted, err)
	}
	action, emitted, err = mapper.mouse(wmMouseMove, mouseHookData{Point: point{X: 101, Y: 102}})
	if err != nil || !emitted || action.DeltaX != 1 || action.DeltaY != 2 {
		t.Fatalf("rebased action=%+v emitted=%t error=%v", action, emitted, err)
	}
}

func TestSourceMapperMapsButtonsAndSignedWheel(t *testing.T) {
	mapper := &sourceMapper{}
	tests := []struct {
		message uint32
		data    uint32
		want    control.InputAction
	}{
		{wmLButtonDown, 0, control.InputAction{Kind: control.InputButtonDown, Code: MouseButtonLeft}},
		{wmRButtonUp, 0, control.InputAction{Kind: control.InputButtonUp, Code: MouseButtonRight}},
		{wmXButtonDown, uint32(xButton2) << 16, control.InputAction{Kind: control.InputButtonDown, Code: MouseButtonX2}},
		{wmMouseWheel, mouseDataWithDelta(-120), control.InputAction{Kind: control.InputWheel, DeltaY: -120}},
		{wmMouseHWheel, mouseDataWithDelta(120), control.InputAction{Kind: control.InputWheel, DeltaX: 120}},
	}
	for _, test := range tests {
		action, emitted, err := mapper.mouse(test.message, mouseHookData{MouseData: test.data})
		if err != nil || !emitted || action != test.want {
			t.Fatalf("mouse(%x) action=%+v emitted=%t error=%v", test.message, action, emitted, err)
		}
	}
}

func mouseDataWithDelta(delta int16) uint32 {
	return uint32(uint16(delta)) << 16
}

func TestSourceMapperRejectsUnknownXButtonAndIgnoresTaggedInput(t *testing.T) {
	mapper := &sourceMapper{}
	_, emitted, err := mapper.mouse(wmXButtonDown, mouseHookData{MouseData: 3 << 16})
	if emitted || !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("unknown X button emitted=%t error=%v", emitted, err)
	}
	_, emitted, err = mapper.mouse(wmLButtonDown, mouseHookData{ExtraInfo: krasisInjectedExtraInfo})
	if err != nil || emitted {
		t.Fatalf("tagged input emitted=%t error=%v", emitted, err)
	}
}
