package handoff

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"
)

func collected(t *testing.T) (*Collector, context.Context, *[]Summary) {
	t.Helper()
	var summaries []Summary
	c := New(func(s Summary) {
		if !s.Valid() {
			t.Errorf("invalid summary: %+v", s)
		}
		summaries = append(summaries, s)
	})
	t.Cleanup(func() {
		if c.timer != nil {
			c.timer.Stop()
		}
	})
	return c, WithSession(context.Background(), c), &summaries
}

func TestFirstFailureAndWorstVectorRemainOneOperation(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "session", "epoch", time.Now())
	Backend(p, "installed")
	Record(p, ReadyWait, 80*time.Millisecond)
	x, complete := Input(ctx, "receiver")
	Record(x, ClientResponse, 30*time.Millisecond)
	Worker(x, [3]byte{1, 2, 3})
	Record(x, ClientResponse, time.Second) // recovery must not overwrite
	Worker(x, [3]byte{5, 5, 5})
	complete(errors.New("test failure"))
	x, complete = Input(ctx, "receiver")
	Record(x, ClientResponse, 7*time.Millisecond)
	x.Value(sampleKey{}).(*sample).started = time.Now().Add(-50 * time.Millisecond)
	complete(nil)
	x, complete = Input(ctx, "receiver")
	Record(x, ClientResponse, time.Second) // independent high stage, shorter dispatch
	x.Value(sampleKey{}).(*sample).started = time.Now().Add(-time.Millisecond)
	complete(nil)
	Finish(p, "ok")
	s := (*out)[0]
	if s.First.Outcome != "error" || s.First.Micros[ClientResponse] != 30000 || s.First.Worker != [3]byte{1, 2, 3} {
		t.Fatalf("first failure replaced: %+v", s.First)
	}
	if s.Frames != 2 || s.Worst.Micros[ClientResponse] != 7000 || s.Histogram[2] != 1 {
		t.Fatalf("mixed vectors or incorrect frame histogram: %+v", s)
	}
}

func TestLateOldSampleAndMismatchedObserverCannotChangeNewAttempt(t *testing.T) {
	_, ctx, out := collected(t)
	old := Begin(ctx, "receiver", "old-session", "old-epoch", time.Now())
	x, complete := Input(ctx, "receiver")
	Finish(old, "ok")
	current := Begin(ctx, "receiver", "new-session", "new-epoch", time.Now())
	Record(x, ClientResponse, time.Second)
	complete(nil)
	Record(Matching(ctx, "receiver", "old-session", "old-epoch"), ReadyWait, time.Second)
	Record(current, ReadyWait, 2*time.Millisecond)
	Finish(current, "ok")
	if len(*out) != 2 || (*out)[1].First.Outcome != "" || (*out)[1].Preparation.Micros[ReadyWait] != 2000 {
		t.Fatalf("late data contaminated new attempt: %+v", *out)
	}
}

func TestFailedAttemptsConsumeProcessBudgetAcrossSessions(t *testing.T) {
	c, ctx, out := collected(t)
	for i := 0; i < Limit+4; i++ {
		ctx = WithSession(ctx, c)
		p := Begin(ctx, "source", "session", "epoch", time.Now())
		Finish(p, "error")
	}
	if len(*out) != Limit || c.count != Limit {
		t.Fatal("failure/reconnect reset the budget")
	}
}

func TestExpiryFlushesIncompleteAndStopsSampling(t *testing.T) {
	c, ctx, out := collected(t)
	p := Begin(ctx, "source", "session", "epoch", time.Now())
	x, complete := Input(ctx, "source")
	c.started = time.Now().Add(-Window)
	Record(p, CaptureActivate, time.Second)
	c.expire()
	Record(x, ClientResponse, time.Second)
	complete(nil)
	Finish(p, "ok")
	if len(*out) != 1 || !(*out)[0].Incomplete || (*out)[0].Preparation.Measured != 0 {
		t.Fatalf("expiry not bounded: %+v", *out)
	}
	if Offered(ctx, time.Now()) != ctx {
		t.Fatal("expired source still allocates offer context")
	}
	if Begin(ctx, "source", "next", "epoch", time.Now()) != ctx {
		t.Fatal("expired window restarted")
	}
}

func TestCountersSaturateAndObserverPanicIsContained(t *testing.T) {
	c, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "session", "epoch", time.Now())
	_, complete := Input(ctx, "receiver")
	complete(nil)
	a := getAttempt(p)
	a.summary.Frames = ^uint32(0)
	a.summary.Histogram[0] = ^uint32(0)
	_, complete = Input(ctx, "receiver")
	complete(nil)
	Finish(p, "ok")
	if !(*out)[0].Incomplete || (*out)[0].Frames != ^uint32(0) {
		t.Fatal("counter wrapped")
	}
	c.emit = func(Summary) { panic("test observer") }
	p = Begin(ctx, "receiver", "next", "epoch", time.Now())
	Finish(p, "ok")
}

func TestSchemaRejectsUnmeasuredValuesAndArbitraryStrings(t *testing.T) {
	base := Summary{Schema: 1, Attempt: 1, Role: "source", Backend: "none", End: "ok"}
	for _, mutate := range []func(*Summary){
		func(s *Summary) { s.Role = "private-marker" },
		func(s *Summary) { s.Backend = "private-marker" },
		func(s *Summary) { s.First.Micros[ClientWrite] = 1 },
		func(s *Summary) { s.First.Worker[0] = 6 },
		func(s *Summary) { s.First.Measured = 1 << StageCount },
		func(s *Summary) { s.First.Outcome = "private-marker" },
		func(s *Summary) { s.Histogram[0] = 1 },
	} {
		s := base
		mutate(&s)
		if s.Valid() {
			t.Fatal("invalid profile accepted")
		}
	}
}

func TestRejectedContendingRequestDoesNotReplaceActiveReceiver(t *testing.T) {
	_, ctx, out := collected(t)
	active := Begin(ctx, "receiver", "active", "epoch", time.Now())
	rejected := Begin(ctx, "receiver", "rejected", "epoch", time.Now())
	Finish(rejected, "error")
	x, complete := Input(ctx, "receiver")
	Record(x, ClientResponse, time.Millisecond)
	complete(nil)
	Finish(active, "ok")
	if len(*out) != 2 || (*out)[0].Attempt != 2 || (*out)[1].Attempt != 1 || (*out)[1].First.Outcome != "ok" {
		t.Fatalf("rejection replaced active receiver: %+v", *out)
	}
}

func TestRetirementFreezesBeforeNextAttemptAndEmitsOutsideCoordinator(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "source", "first", "epoch", time.Now())
	_, complete := Input(ctx, "source")
	complete(nil)
	emit := Retire(p, "ok")
	if len(*out) != 0 {
		t.Fatal("retirement emitted inside caller's critical section")
	}
	next := Begin(ctx, "source", "next", "epoch", time.Now())
	emit()
	if len(*out) != 1 || (*out)[0].Attempt != 1 || (*out)[0].Incomplete || (*out)[0].End != "ok" {
		t.Fatalf("rapid reentry corrupted retirement: %+v", *out)
	}
	Finish(next, "error")
}

func TestExhaustedBudgetCannotAppendNewLeaseToLastAttempt(t *testing.T) {
	for _, role := range []string{"source", "receiver"} {
		t.Run(role, func(t *testing.T) {
			_, ctx, out := collected(t)
			for i := 1; i < Limit; i++ {
				p := Begin(ctx, role, strconv.Itoa(i), "epoch", time.Now())
				Finish(p, "ok")
			}
			last := Begin(ctx, role, "last", "epoch", time.Now())
			_, complete := Input(ctx, role)
			complete(nil)
			Begin(ctx, role, "untracked", "next-epoch", time.Now())
			x, complete := Input(ctx, role)
			Record(x, ClientResponse, time.Second)
			complete(nil)
			Finish(last, "incomplete")
			if len(*out) != Limit || (*out)[Limit-1].Frames != 0 || (*out)[Limit-1].Worst.Measured != 0 {
				t.Fatalf("untracked lease appended to last attempt: %+v", (*out)[len(*out)-1])
			}
		})
	}
}
