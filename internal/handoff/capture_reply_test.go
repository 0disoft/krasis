package handoff

import "testing"

func TestCaptureReplyPartitionsOnlyObservedOrderedLocalClocks(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		read, publish, resume uint64
		measured              bool
	}{
		{"missing", 0, 20, 30, false},
		{"before_read", 20, 19, 30, false},
		{"before_publish", 20, 30, 29, false},
		{"zero_span", 20, 20, 20, true},
		{"ordinary", 20, 30, 100, true},
		{"below_window", 1, 2, uint64(Window.Microseconds()), true},
		{"at_window", 1, 2, uint64(Window.Microseconds()) + 1, true},
		{"past_window", 1, 2, uint64(Window.Microseconds()) + 2, false},
		{"overflow", 1, 2, ^uint64(0), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := CaptureReplyFromClocks(tc.read, tc.publish, tc.resume)
			if p.Measured != tc.measured || !tc.measured && p != (CaptureReply{}) {
				t.Fatal(p)
			}
			if tc.measured && !p.valid(uint32(tc.resume-tc.read)) {
				t.Fatal("lost adjacent sum", p)
			}
		})
	}
	p := CaptureIPCFromClocks([9]uint64{1, 2, 3, 4, 5, 6, 7, 8, 80})
	p.Reply = CaptureReplyFromClocks(8, 20, 80)
	if !p.Valid() {
		t.Fatal(p)
	}
	p.Reply.PublishToResumeMicros++
	if p.Valid() {
		t.Fatal("inconsistent subdivision accepted")
	}
	p.Reply = CaptureReply{ReadToPublishMicros: 1}
	if p.Valid() {
		t.Fatal("unmeasured data accepted")
	}
}
