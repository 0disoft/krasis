package handoff

import (
	"testing"
	"time"
)

func TestPreparationLockDetailRequiresMeasuredReceiverOpenLock(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "session", "epoch", time.Now())
	RecordPreparationLock(p, time.Millisecond)
	if getAttempt(p).summary.PreparationLockMeasured {
		t.Fatal("detail attached without an OpenLock observation")
	}
	Record(p, OpenLock, 10*time.Millisecond)
	RecordPreparationLock(p, 11*time.Millisecond)
	RecordPreparationLock(p, -time.Millisecond)
	if getAttempt(p).summary.PreparationLockMeasured {
		t.Fatal("impossible overlap became measured")
	}
	RecordPreparationLock(p, 6*time.Millisecond)
	RecordPreparationLock(p, time.Millisecond)
	Finish(p, "ok")
	RecordPreparationLock(p, 0)
	s := (*out)[0]
	if !s.PreparationLockMeasured || s.PreparationLockMicros != 6000 {
		t.Fatal("first overlap lost or retired summary changed", s)
	}
	for _, mutate := range []func(*Summary){
		func(s *Summary) { s.PreparationLockMeasured = false },
		func(s *Summary) { s.PreparationLockMicros = 10001 },
		func(s *Summary) { s.Role, s.Backend = "source", "none" },
		func(s *Summary) { s.Preparation.Measured = 0; s.Preparation.Micros[OpenLock] = 0 },
	} {
		bad := s
		mutate(&bad)
		if bad.Valid() {
			t.Fatal("invalid overlap profile accepted", bad)
		}
	}
}

func TestPreparationLockZeroAndExpiryStayExplicit(t *testing.T) {
	c, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "session", "epoch", time.Now())
	Record(p, OpenLock, time.Millisecond)
	RecordPreparationLock(p, 0)
	Finish(p, "ok")
	if !(*out)[0].PreparationLockMeasured || (*out)[0].PreparationLockMicros != 0 {
		t.Fatal("observed absence of overlap lost")
	}
	p = Begin(ctx, "receiver", "next", "epoch", time.Now())
	Record(p, OpenLock, time.Millisecond)
	c.started = time.Now().Add(-Window)
	RecordPreparationLock(p, time.Millisecond)
	if getAttempt(p).summary.PreparationLockMeasured {
		t.Fatal("expired window sampled overlap")
	}
	EndSession(ctx)
}
