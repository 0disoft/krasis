package productlog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestFocusExportProjectsOnlyBoundedNumericStatesAndResults(t *testing.T) {
	p := &handoff.FocusProfile{Schema: 1, States: [4]uint8{3, 2, 3, 1}, Observed: 15, Restore: 2, PostHide: 2, ResultMeasured: 3, Measured: 3}
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "source", Backend: "none", End: "ok", Focus: p}
	r := record{Time: "2026-10-05T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"focus":{`, `"focus":{"window":"private-marker","title":"private-marker",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.Focus == nil || *got.Events[0].Handoff.Focus != *p || strings.Contains(string(encoded), "private-marker") {
		t.Fatal("focus projection leaked or lost evidence")
	}
	p.Observed = 7
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unobserved focus state exported")
	}
}

func TestHandoffExportReprojectsFixedSchemaAndDropsIdentity(t *testing.T) {
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "incomplete", Incomplete: true}
	s.Preparation.Micros[handoff.ReadyWait] = 1000
	s.Preparation.Measured = 1 << handoff.ReadyWait
	s.First.Worker = [3]byte{1, 4, 2}
	s.First.WorkerMillis = [3]uint16{0, 197, 4}
	s.First.WorkerMeasured = 7
	s.First.AuthorityMicros = [3]uint32{0, 4, 15000}
	s.First.AuthorityMeasured = 7
	s.First.PipeOwnerCycles, s.First.PipeOwnerCyclesMeasured = 1234, true
	r := record{Time: "2026-10-03T00:00:00Z", Code: "handoff_profile", Connection: strings.Repeat("a", 64), Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"handoff":{`, `"handoff":{"pin":"secret-marker","path":"private-path",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff == nil || *got.Events[0].Handoff != s {
		t.Fatalf("valid summary lost: %+v", got)
	}
	if strings.Contains(string(encoded), "secret-marker") || strings.Contains(string(encoded), "private-path") || strings.Contains(string(encoded), "connection") {
		t.Fatal("private data exported")
	}
	s.First.WorkerMeasured = 1
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unobserved worker milliseconds exported")
	}
	s.First.WorkerMeasured = 7
	s.First.AuthorityMeasured = 1
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unobserved authority microseconds exported")
	}
	s.First.AuthorityMeasured = 7
	s.First.PipeOwnerCycles = handoff.MaxPipeOwnerCycles + 1
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("out-of-range pipe owner cycles exported")
	}
	s.First.PipeOwnerCycles = 1234
	s.Backend = "secret-marker"
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("arbitrary profile string accepted")
	}
}

func TestStartupExportReprojectsNumericSchemaAndRejectsInvalidValues(t *testing.T) {
	profile := &handoff.StartupProfile{Schema: 1, Measured: 1 | 1<<handoff.WorkerInstalledValidation}
	profile.Micros[handoff.ServiceLaunch] = 1200
	profile.Micros[handoff.WorkerInstalledValidation] = 2000
	profile.RecordLoad(time.Millisecond)
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "ok", Startup: profile}
	r := record{Time: "2026-10-03T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"startup":{`, `"startup":{"pin":"secret-marker","path":"private-path",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.Startup == nil || *got.Events[0].Handoff.Startup != *profile {
		t.Fatal("validated startup diagnostics lost")
	}
	if strings.Contains(string(encoded), "secret-marker") || strings.Contains(string(encoded), "private-path") {
		t.Fatal("private startup fields exported")
	}
	profile.Micros[handoff.WorkerIdentity] = 1
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unmeasured startup value accepted by export")
	}
}

func TestCaptureExportProjectsNumericDetailAndRejectsInvalidPresence(t *testing.T) {
	p := &handoff.CaptureProfile{Schema: 1, Measured: 1 << handoff.CaptureShow}
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "source", Backend: "none", End: "incomplete", Incomplete: true, Capture: p}
	r := record{Time: "2026-10-04T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"capture":{`, `"capture":{"window":"private-marker",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.Capture == nil || *got.Events[0].Handoff.Capture != *p || strings.Contains(string(encoded), "private-marker") {
		t.Fatal("capture projection leaked or lost evidence")
	}
	p.Micros[handoff.CaptureFallback] = 1
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unobserved capture timings exported")
	}
}

func TestExchangeExportProjectsOnlyValidNumericDetail(t *testing.T) {
	p := &handoff.ExchangeProfile{Schema: 1, Measured: 3, Micros: [handoff.ExchangeStageCount]uint32{15000, 1000}}
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "source", Backend: "none", End: "incomplete", Incomplete: true, Exchange: p}
	r := record{Time: "2026-10-05T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"exchange":{`, `"exchange":{"pin":"private-marker","session":"private-id",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.Exchange == nil || *got.Events[0].Handoff.Exchange != *p || strings.Contains(string(encoded), "private-") {
		t.Fatal("exchange projection leaked or lost evidence")
	}
	p.Measured |= 1 << handoff.RequestReadToHandler
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("foreign-role exchange exported")
	}
}

func TestPreparationLockExportPreservesPresenceAndBounds(t *testing.T) {
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "incomplete", Incomplete: true, PreparationLockMeasured: true, PreparationLockMicros: 4000}
	s.Preparation.Measured = 1 << handoff.OpenLock
	s.Preparation.Micros[handoff.OpenLock] = 5000
	r := record{Time: "2026-10-05T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"handoff":{`, `"handoff":{"preparationOwner":"private-marker","preparationStarted":"private-time",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || !got.Events[0].Handoff.PreparationLockMeasured || got.Events[0].Handoff.PreparationLockMicros != 4000 || strings.Contains(string(encoded), "private-") {
		t.Fatal("numeric overlap lost or private fields exported")
	}
	s.PreparationLockMicros = 6000
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("overlap greater than the measured wait exported")
	}
}
