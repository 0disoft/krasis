package handoff

import "testing"

func TestCaptureRequestClockOrderAndLimits(t *testing.T) {
	limit := uint64(Window.Microseconds())
	for _, tc := range []struct {
		name                            string
		sent, submit, returned, receive uint64
		valid                           bool
	}{
		{"zero duration", 100, 100, 100, 100, true},
		{"reader before write return", 100, 110, 150, 120, true},
		{"reader after write return", 100, 110, 120, 150, true},
		{"missing", 0, 110, 120, 150, false},
		{"submission reversed", 100, 99, 120, 150, false},
		{"return reversed", 100, 110, 109, 150, false},
		{"receive reversed", 100, 110, 120, 109, false},
		{"before limit", 100, 110, 120, 100 + limit - 1, true},
		{"at limit", 100, 110, 100 + limit, 100 + limit, true},
		{"receive past limit", 100, 110, 120, 101 + limit, false},
		{"return past limit", 100, 110, 101 + limit, 120, false},
		{"overflow", 100, 110, 120, ^uint64(0), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := CaptureRequestFromClocks(tc.sent, tc.submit, tc.returned, tc.receive)
			if p.Measured != tc.valid {
				t.Fatal(p)
			}
			if tc.valid && (!p.valid(uint32(tc.receive-tc.sent)) || p.SubmitMicros != uint32(tc.submit-tc.sent) || p.ReceiveMicros != uint32(tc.receive-tc.submit) || p.WriteCompletionMicros != uint32(tc.returned-tc.submit)) {
				t.Fatal(p)
			}
			if !tc.valid && p != (CaptureRequest{}) {
				t.Fatal(p)
			}
		})
	}
	parent := CaptureIPCFromClocks([9]uint64{100, 130, 130, 130, 130, 130, 130, 130, 130})
	parent.Request = CaptureRequestFromClocks(100, 110, 120, 130)
	if !parent.Valid() {
		t.Fatal(parent)
	}
	parent.Request.ReceiveMicros++
	if parent.Valid() {
		t.Fatal("nested span may not disagree with delivery")
	}
}
