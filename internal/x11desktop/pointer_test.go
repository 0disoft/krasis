package x11desktop

import (
	"github.com/jezek/xgb"
	"testing"
)

func TestFullPointerMaskIncludesSideButtons(t *testing.T) {
	r := make([]byte, 60)
	xgb.Put16(r[34:], 1)
	for _, button := range []int{1, 2, 3, 8, 9, 31} {
		r[56+button/8] = 1 << uint(button%8)
		held, err := pointerMaskPressed(r)
		if err != nil || !held {
			t.Fatal(button, held, err)
		}
		r[56+button/8] = 0
	}
	if held, err := pointerMaskPressed(r); err != nil || held {
		t.Fatal(held, err)
	}
	for _, bad := range [][]byte{r[:55], r[:59]} {
		if _, err := pointerMaskPressed(bad); err == nil {
			t.Fatal("truncated mask accepted")
		}
	}
}
