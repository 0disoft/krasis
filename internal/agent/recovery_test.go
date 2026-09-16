package agent

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestRecoveryDeadlineReportsUnknownWithoutReleasingOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		joined := make(chan struct{})
		reports := make(chan RecoveryReport, 1)
		done := make(chan bool, 1)
		go func() { done <- awaitRecovery(joined, time.Second, func(r RecoveryReport) { reports <- r }) }()
		report := <-reports
		if !report.DeadlineExceeded || report.WorkersJoined != RecoveryUnknown || report.PressedLedgerEmpty != RecoveryUnknown {
			t.Fatalf("false proof: %+v", report)
		}
		select {
		case <-done:
			t.Fatal("unfinished owner released")
		default:
		}
		close(joined)
		if !<-done {
			t.Fatal("lost exceeded deadline")
		}
	})
}

func TestRecoveryCleanJoinDoesNotFault(t *testing.T) {
	joined := make(chan struct{})
	close(joined)
	if awaitRecovery(joined, time.Second, func(RecoveryReport) { t.Fatal("clean join reported timeout") }) {
		t.Fatal("clean join timed out")
	}
}
