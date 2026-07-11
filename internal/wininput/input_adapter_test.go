package wininput

import (
	"errors"
	"testing"
)

func TestScanCodeEncodingRoundTrip(t *testing.T) {
	for _, extended := range []bool{false, true} {
		code, err := EncodeScanCode(0x1D, extended)
		if err != nil {
			t.Fatalf("EncodeScanCode() error = %v", err)
		}
		scanCode, gotExtended, err := decodeScanCode(code)
		if err != nil {
			t.Fatalf("decodeScanCode() error = %v", err)
		}
		if scanCode != 0x1D || gotExtended != extended {
			t.Fatalf("decoded = (%x, %t)", scanCode, gotExtended)
		}
	}
}

func TestScanCodeEncodingRejectsZeroAndReservedBits(t *testing.T) {
	if _, err := EncodeScanCode(0, false); !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("EncodeScanCode(0) error = %v", err)
	}
	if _, _, err := decodeScanCode(1 << 31); !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("decodeScanCode(reserved) error = %v", err)
	}
}

func TestMouseButtonValidationIsClosed(t *testing.T) {
	for code := MouseButtonLeft; code <= MouseButtonX2; code++ {
		if err := validateMouseButton(code); err != nil {
			t.Fatalf("validateMouseButton(%d) error = %v", code, err)
		}
	}
	if err := validateMouseButton(0); !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("validateMouseButton(0) error = %v", err)
	}
	if err := validateMouseButton(MouseButtonX2 + 1); !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("validateMouseButton(6) error = %v", err)
	}
}
