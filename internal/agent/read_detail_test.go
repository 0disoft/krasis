package agent

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type detailTestConnection struct {
	Connection
	detail func(int, time.Time)
	ready  bool
}

func TestConfiguredGrantReadDetailIsScopedToTheCurrentConnectionSession(t *testing.T) {
	var summaries []handoff.Summary
	collector := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	first := handoff.WithSession(context.Background(), collector)
	raw := &detailTestConnection{ready: true}
	configureGrantReadProgress(first, &supervisedConnection{Connection: raw})
	oldCallback := raw.detail
	p := handoff.Begin(first, "source", "first", "epoch", time.Now())
	oldCallback(12, time.Now())
	handoff.Granted(handoff.WithControlRead(p, time.Now()))
	handoff.Finish(p, "ok")
	handoff.EndSession(first)
	next := handoff.WithSession(context.Background(), collector)
	configureGrantReadProgress(next, &supervisedConnection{Connection: raw})
	p = handoff.Begin(next, "source", "next", "epoch", time.Now())
	oldCallback(100, time.Now())
	raw.detail(24, time.Now())
	handoff.Granted(handoff.WithControlRead(p, time.Now()))
	handoff.Finish(p, "ok")
	handoff.EndSession(next)
	if len(summaries) != 2 || !summaries[0].Valid() || !summaries[1].Valid() || summaries[0].GrantProgress == nil || summaries[1].GrantProgress == nil {
		t.Fatal("configured grant progress missing")
	}
	if summaries[0].GrantProgress.Bytes != 12 || summaries[1].GrantProgress.Bytes != 24 {
		t.Fatal("old connection callback contaminated the reconnected attempt")
	}
}

func (c *detailTestConnection) SetReadDetailObserver(f func(int, time.Time)) bool {
	c.detail = f
	return c.ready
}

func TestSupervisedReadDetailForwardsRealAvailability(t *testing.T) {
	for _, ready := range []bool{false, true} {
		raw := &detailTestConnection{ready: ready}
		wrapper := &supervisedConnection{Connection: raw}
		bytes := 0
		if wrapper.SetReadDetailObserver(func(n int, _ time.Time) { bytes += n }) != ready || raw.detail == nil {
			t.Fatal("supervisor invented or hid the byte observer")
		}
		raw.detail(12, time.Now())
		if bytes != 12 {
			t.Fatal("supervisor lost detail callback")
		}
	}
	if (&supervisedConnection{}).SetReadDetailObserver(nil) {
		t.Fatal("absent connection exposed a byte observer")
	}
}
