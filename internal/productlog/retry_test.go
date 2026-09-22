package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRetryPolicyExportPreservesUnlimitedAndActualAttempt(t *testing.T) {
	l := &Logger{queue: make(chan record, 1)}
	s := RetrySummary{Policy: "agent-exponential-v1", Unlimited: true, Attempt: 4, NextMillis: 1900, InitialMillis: 250, MaximumMillis: 30000, StableMillis: 30000, StableReset: true, JitterPercent: 20}
	l.Retry(s)
	raw, _ := json.Marshal(<-l.queue)
	exported := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
	if len(exported.Events) != 1 || exported.Events[0].Retry == nil || *exported.Events[0].Retry != s {
		t.Fatalf("policy lost: %s", raw)
	}
}
