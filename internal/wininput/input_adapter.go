package wininput

import (
	"errors"
	"fmt"

	"github.com/0disoft/krasis/internal/control"
)

const (
	MouseButtonLeft   uint32 = 1
	MouseButtonRight  uint32 = 2
	MouseButtonMiddle uint32 = 3
	MouseButtonX1     uint32 = 4
	MouseButtonX2     uint32 = 5

	extendedScanCodeFlag uint32 = 1 << 16
	scanCodeMask         uint32 = 0xFFFF
	cursorEntryInset            = 1
)

var (
	ErrUnsupportedInput = errors.New("unsupported Windows input")
	ErrInputInjection   = errors.New("Windows input injection failed")
	ErrCursorEntryState = errors.New("Windows cursor entry state is uncertain")
)

func EncodeScanCode(scanCode uint16, extended bool) (uint32, error) {
	if scanCode == 0 {
		return 0, fmt.Errorf("%w: scan code must not be zero", ErrUnsupportedInput)
	}
	code := uint32(scanCode)
	if extended {
		code |= extendedScanCodeFlag
	}
	return code, nil
}

func cursorEntryPoint(
	sourceEdge control.ScreenEdge,
	left int32,
	top int32,
	width int32,
	height int32,
) (int32, int32, error) {
	if sourceEdge != control.ScreenEdgeLeft && sourceEdge != control.ScreenEdgeRight {
		return 0, 0, fmt.Errorf("%w: source edge must be LEFT or RIGHT", ErrUnsupportedInput)
	}
	if width <= cursorEntryInset*2 || height <= 0 {
		return 0, 0, fmt.Errorf("%w: virtual screen dimensions are invalid", ErrUnsupportedInput)
	}
	right := int64(left) + int64(width) - 1
	bottom := int64(top) + int64(height) - 1
	if right > int64(^uint32(0)>>1) || right < -int64(^uint32(0)>>1)-1 ||
		bottom > int64(^uint32(0)>>1) || bottom < -int64(^uint32(0)>>1)-1 {
		return 0, 0, fmt.Errorf("%w: virtual screen coordinates overflow int32", ErrUnsupportedInput)
	}
	x := left + cursorEntryInset
	if sourceEdge == control.ScreenEdgeLeft {
		x = int32(right) - cursorEntryInset
	}
	y := int32(int64(top) + int64(height)/2)
	return x, y, nil
}

func decodeScanCode(code uint32) (uint16, bool, error) {
	if code&^(scanCodeMask|extendedScanCodeFlag) != 0 || code&scanCodeMask == 0 {
		return 0, false, fmt.Errorf("%w: invalid encoded scan code", ErrUnsupportedInput)
	}
	return uint16(code & scanCodeMask), code&extendedScanCodeFlag != 0, nil
}

func validateMouseButton(code uint32) error {
	switch code {
	case MouseButtonLeft, MouseButtonRight, MouseButtonMiddle, MouseButtonX1, MouseButtonX2:
		return nil
	default:
		return fmt.Errorf("%w: mouse button code is not supported", ErrUnsupportedInput)
	}
}
