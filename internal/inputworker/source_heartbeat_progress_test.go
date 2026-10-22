package inputworker

import (
	"bytes"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/logindiag"
)

func TestHeartbeatProgressRetainsSlowStageAcrossLaterReplies(t *testing.T) {
	var progress sourceHeartbeatProgress
	progress.record(1, 150*time.Millisecond)
	progress.record(5, time.Millisecond)
	progress.stage = 6
	exit := cleanSourceExit()
	progress.snapshot(&exit)
	var wire bytes.Buffer
	if err := writeReport(&wire, Report{Source: &exit}, nil); err != nil {
		t.Fatal(err)
	}
	if wire.Bytes()[4] != 4 || wire.Len() != reportSize {
		t.Fatal("heartbeat diagnostics changed framing or used legacy version")
	}
	report, err := readReport(&wire)
	if err != nil || report.Source == nil || *report.Source != exit || exit.HeartbeatStage != "replied" || exit.HeartbeatSlowStage != "service_presence" || exit.HeartbeatSlowBucket != 4 {
		t.Fatal("lost slow stage or final progress", report, err)
	}
}

func TestHeartbeatProgressRejectsUnknownAndContradictoryWireValues(t *testing.T) {
	exit := cleanSourceExit()
	exit.HeartbeatStage, exit.HeartbeatSlowStage, exit.HeartbeatSlowBucket = "reply_write", "reply_gate", 5
	good, err := encodeSourceExit(exit, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]byte{{255, good[7]}, {good[6], 0xf1}, {good[6], 0x16}, {good[6], 0x01}, {0, good[7]}, {6, 0x61}} {
		bad := good
		bad[6], bad[7] = pair[0], pair[1]
		if _, err := decodeSourceExit(bad[:], 1); err != ErrProtocol {
			t.Fatal("invalid progress accepted", pair, err)
		}
	}
	// Existing runtime documents and source reports remain valid without it.
	var empty sourceHeartbeatProgress
	legacy := cleanSourceExit()
	empty.snapshot(&legacy)
	if !legacy.ValidDetails() || legacy.HeartbeatStage != "" {
		t.Fatal(legacy)
	}
	private := legacy
	private.HeartbeatStage = "PRIVATE/input"
	if private.ValidDetails() {
		t.Fatal("private stage accepted")
	}
	missing := logindiag.SourceExit{Report: "missing", HeartbeatStage: "replied"}
	if missing.ValidDetails() {
		t.Fatal("missing report invented progress")
	}
}
