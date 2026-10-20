package wininput

import "testing"

func TestExternalCaptureNeverErasesHeldInputOrChangesActiveAuthority(t *testing.T) {
	session := &SourceSession{capturePaused: true, physicalCheck: func() bool { return true }}
	if !session.UseExternalPhysicalCapture() || session.physicalCheck != nil {
		t.Fatal("obsolete medium desktop check retained")
	}
	session.physicalHeld = map[physicalPress]struct{}{{code: 1}: {}}
	if session.UseExternalPhysicalCapture() || len(session.physicalHeld) != 1 {
		t.Fatal("physical pressed journal erased")
	}
	session.physicalHeld = nil
	session.active.Store(true)
	if session.UseExternalPhysicalCapture() {
		t.Fatal("active forwarding authority changed")
	}
}
