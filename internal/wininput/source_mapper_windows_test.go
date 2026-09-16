//go:build windows

package wininput

import (
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceMapperMapsPhysicalScanCodes(t *testing.T) {
	mapper := &sourceKeyboardMapper{}
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
	mapper := &sourceKeyboardMapper{}
	_, emitted, err := mapper.keyboard(wmKeyDown, keyboardHookData{})
	if emitted || !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("keyboard(zero) emitted=%t error=%v", emitted, err)
	}
}

func TestRightShiftHasStableIdentityAcrossIMEExtendedFlags(t *testing.T) {
	mapper := &sourceKeyboardMapper{}
	for _, message := range []uint32{wmKeyDown, wmKeyUp, wmSysKeyDown, wmSysKeyUp} {
		for _, flags := range []uint32{0, llkhfExtended} {
			action, emitted, err := mapper.keyboard(message, keyboardHookData{VirtualKey: 0xA1, ScanCode: 0x36, Flags: flags})
			if err != nil || !emitted || action.Code != 0x36 {
				t.Fatalf("right shift message=%x flags=%x action=%+v emitted=%v error=%v", message, flags, action, emitted, err)
			}
			want := control.InputKeyDown
			if message == wmKeyUp || message == wmSysKeyUp {
				want = control.InputKeyUp
			}
			if action.Kind != want {
				t.Fatalf("kind=%v want=%v", action.Kind, want)
			}
		}
	}
}
