package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type handoffTagConnection struct {
	stubConnection
	tag   string
	calls int
}

func (c *handoffTagConnection) DiagnosticExchangeTag(string, string) string { c.calls++; return c.tag }

func TestHandoffCorrelationForwardedAndReboundOnReconnect(t *testing.T) {
	var summaries []handoff.Summary
	collector := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	for _, tag := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		ctx := handoff.WithSession(context.Background(), collector)
		raw := &handoffTagConnection{tag: tag}
		configureHandoffCorrelation(ctx, &supervisedConnection{Connection: raw})
		p := handoff.Begin(ctx, "source", "s", "e", time.Now())
		handoff.Finish(p, "ok")
		handoff.EndSession(ctx)
		if raw.calls != 1 || summaries[len(summaries)-1].ExchangeTag != tag {
			t.Fatal("supervisor hid tag or reused old connection")
		}
	}
	for _, missing := range []*supervisedConnection{nil, {}, {Connection: &stubConnection{}}} {
		if missing.DiagnosticExchangeTag("s", "e") != "" {
			t.Fatal("missing capability invented tag")
		}
	}
}
