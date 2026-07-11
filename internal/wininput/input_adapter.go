package wininput

import (
	"errors"
	"fmt"
)

const (
	MouseButtonLeft   uint32 = 1
	MouseButtonRight  uint32 = 2
	MouseButtonMiddle uint32 = 3
	MouseButtonX1     uint32 = 4
	MouseButtonX2     uint32 = 5

	extendedScanCodeFlag uint32 = 1 << 16
	scanCodeMask         uint32 = 0xFFFF
)

var (
	ErrUnsupportedInput = errors.New("unsupported Windows input")
	ErrInputInjection   = errors.New("Windows input injection failed")
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
