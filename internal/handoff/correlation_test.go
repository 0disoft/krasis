package handoff

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExchangeTagsIgnoreLocalOrdinalAndStayBounded(t *testing.T) {
	_, left, leftOut := collected(t)
	_, right, rightOut := collected(t)
	tag := strings.Repeat("a", 64)
	leftCalls, rightCalls := 0, 0
	ConfigureExchangeTags(left, func(s, e string) string {
		leftCalls++
		if s == "s" && e == "e" {
			return tag
		}
		return strings.Repeat("b", 64)
	})
	ConfigureExchangeTags(right, func(s, e string) string {
		rightCalls++
		if s == "s" && e == "e" {
			return tag
		}
		return strings.Repeat("b", 64)
	})
	Finish(Begin(right, "source", "earlier", "e", time.Now()), "ok")
	Finish(Begin(left, "source", "s", "e", time.Now()), "ok")
	Finish(Begin(right, "receiver", "s", "e", time.Now()), "ok")
	if (*leftOut)[0].ExchangeTag != (*rightOut)[1].ExchangeTag || (*leftOut)[0].Attempt == (*rightOut)[1].Attempt {
		t.Fatal("correlation depends on local ordinal")
	}
	for i := 1; i < Limit+1; i++ {
		Finish(Begin(left, "source", "s", "e", time.Now()), "ok")
	}
	if leftCalls != Limit || rightCalls != 2 {
		t.Fatal("untracked attempt derived tag")
	}
}

func TestExchangeTagUnavailableExpiredOrLateDoesNotChangeOutcome(t *testing.T) {
	for _, mode := range []string{"none", "invalid", "expired", "stopped", "retired_during_derivation"} {
		t.Run(mode, func(t *testing.T) {
			c, ctx, out := collected(t)
			calls := 0
			if mode != "none" {
				ConfigureExchangeTags(ctx, func(string, string) string {
					calls++
					if mode == "invalid" {
						return "SECRET"
					}
					if mode == "retired_during_derivation" {
						EndSession(ctx)
					}
					return strings.Repeat("a", 64)
				})
			}
			if mode == "expired" {
				c.started = time.Now().Add(-Window)
			}
			if mode == "stopped" {
				c.expire()
			}
			p := Begin(ctx, "source", "s", "e", time.Now())
			Finish(p, "ok")
			if mode == "expired" || mode == "stopped" {
				if calls != 0 || len(*out) != 0 {
					t.Fatal("inactive collector derived tag")
				}
			} else if len(*out) != 1 || (*out)[0].ExchangeTag != "" || !(*out)[0].Valid() {
				t.Fatal("invalid/late metadata changed outcome")
			}
		})
	}
	for _, invalid := range []string{"", "SECRET", strings.Repeat("a", 63), strings.Repeat("A", 64), strings.Repeat("g", 64)} {
		if ValidExchangeTag(invalid) {
			t.Fatal("invalid tag accepted")
		}
	}
	ConfigureExchangeTags(context.Background(), nil)
}
