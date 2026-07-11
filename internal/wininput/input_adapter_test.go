package wininput

import (
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/control"
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

func TestCursorEntryPointUsesOppositeVirtualScreenEdge(t *testing.T) {
	tests := []struct {
		source control.ScreenEdge
		wantX  int32
	}{
		{source: control.ScreenEdgeRight, wantX: -1919},
		{source: control.ScreenEdgeLeft, wantX: -2},
	}
	for _, test := range tests {
		x, y, err := cursorEntryPoint(test.source, -1920, -100, 1920, 1000)
		if err != nil {
			t.Fatalf("cursorEntryPoint(%s) error = %v", test.source, err)
		}
		if x != test.wantX || y != 400 {
			t.Fatalf("cursorEntryPoint(%s) = (%d,%d), want (%d,400)", test.source, x, y, test.wantX)
		}
	}
}

func TestCursorEntryPointRejectsInvalidGeometry(t *testing.T) {
	if _, _, err := cursorEntryPoint("", 0, 0, 1920, 1080); !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("invalid edge error = %v", err)
	}
	if _, _, err := cursorEntryPoint(control.ScreenEdgeRight, 0, 0, 2, 1080); !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("invalid width error = %v", err)
	}
	if _, _, err := cursorEntryPoint(control.ScreenEdgeRight, 2147483640, 0, 100, 1080); !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("overflow error = %v", err)
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
