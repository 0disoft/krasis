package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/handoff"
	"strings"
	"testing"
	"time"
)

type correlationSource struct{ connection agent.Connection }

func (s correlationSource) Close() error { return nil }

func (s correlationSource) Next(context.Context) (agent.Connection, error) { return s.connection, nil }

type correlatedProductConnection struct {
	disconnectTestConnection
	tag   string
	calls int
}

func (c *correlatedProductConnection) DiagnosticExchangeTag(session, epoch string) string {
	if session != "private-session" || epoch != "private-epoch" {
		panic("incorrect private identity")
	}
	c.calls++
	return c.tag
}
func TestProductConnectionPreservesCorrelationAcrossReconnect(t *testing.T) {
	var summaries []handoff.Summary
	collector := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	for _, tag := range []string{strings.Repeat("a", 64), strings.Repeat("b", 64)} {
		raw := &correlatedProductConnection{tag: tag}
		source := &disconnectSource{ConnectionSource: correlationSource{connection: raw}, intent: context.Background()}
		wrapped, err := source.Next(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		diagnostic, ok := wrapped.(interface{ DiagnosticExchangeTag(string, string) string })
		if !ok {
			t.Fatal("product wrapper hid correlation capability")
		}
		ctx := handoff.WithSession(context.Background(), collector)
		handoff.ConfigureExchangeTags(ctx, diagnostic.DiagnosticExchangeTag)
		attempt := handoff.Begin(ctx, "source", "private-session", "private-epoch", time.Now())
		handoff.Finish(attempt, "ok")
		handoff.EndSession(ctx)
		if raw.calls != 1 || summaries[len(summaries)-1].ExchangeTag != tag {
			t.Fatal("product wrapper lost or reused correlation tag")
		}
		if len(raw.frames) != 0 || raw.closed != 0 {
			t.Fatal("diagnostic mapping changed transport lifecycle")
		}
	}
	for _, missing := range []*disconnectConnection{nil, {}, {Connection: &disconnectTestConnection{}}} {
		if missing.DiagnosticExchangeTag("s", "e") != "" {
			t.Fatal("missing transport invented tag")
		}
	}
}
