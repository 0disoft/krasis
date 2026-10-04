// Package handoff records bounded, process-local software timings. It never
// participates in authority, input outcome classification, or wire protocols.
package handoff

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const Limit = 12
const Window = 120 * time.Second
const MaxWorkerMillis = 1022

type Stage uint8

const (
	BoundaryToBegin Stage = iota
	RequestCoordinator
	RequestWrite
	RequestToGrant
	PeerAuthorize
	AuthorizeTotal
	OpenLock
	ConnectTotal
	PipeOpen
	ServiceAuth
	ServiceImagePin
	LeaseWrite
	ReadyWait
	NewClientCheck
	ReturnArm
	EntryPrepare
	PrepareInbound
	GrantWrite
	GrantCoordinator
	SourceActivate
	CaptureActivate
	CommitWrite
	CommitCoordinator
	CommitApply
	SourceQueue
	FirstOfferSinceBegin
	InputCoordinator
	InputWrite
	Dispatch
	OuterLock
	ClientTotal
	ClientLock
	ClientAuthority
	ClientWrite
	ClientResponse
	CursorPostprocess
	SourceDeactivate
	InputClose
	ReentryGap
	StageCount
)

// Vector preserves one operation, not unrelated per-stage maxima. Worker
// buckets come only from the existing matching ACK (0 means unobserved).
type Vector struct {
	Micros                  [StageCount]int64 `json:"micros"`
	Measured                uint64            `json:"measured"`
	Worker                  [3]byte           `json:"worker"`
	WorkerMillis            [3]uint16         `json:"workerMillis"`
	WorkerMeasured          uint8             `json:"workerMeasured"`
	AuthorityMicros         [3]uint32         `json:"authorityMicros"`
	AuthorityMeasured       uint8             `json:"authorityMeasured"`
	PipeOwnerCycles         uint64            `json:"pipeOwnerCycles,omitempty"`
	PipeOwnerCyclesMeasured bool              `json:"pipeOwnerCyclesMeasured,omitempty"`
	Outcome                 string            `json:"outcome"`
}

type Summary struct {
	Schema      uint8           `json:"schema"`
	Attempt     uint8           `json:"attempt"`
	Role        string          `json:"role"`
	Backend     string          `json:"backend"`
	Preparation Vector          `json:"preparation"`
	First       Vector          `json:"first"`
	Worst       Vector          `json:"worst"`
	Frames      uint32          `json:"frames"`
	Histogram   [5]uint32       `json:"histogram"`
	End         string          `json:"end"`
	Incomplete  bool            `json:"incomplete"`
	Startup     *StartupProfile `json:"startup,omitempty"`
	Capture     *CaptureProfile `json:"capture,omitempty"`
}

func (s Summary) Valid() bool {
	if s.Schema != 1 || s.Attempt < 1 || s.Attempt > Limit || (s.Role != "source" && s.Role != "receiver") ||
		(s.Backend != "none" && s.Backend != "native" && s.Backend != "installed") || !validOutcome(s.End) {
		return false
	}
	if (s.Role == "source") != (s.Backend == "none") {
		return false
	}
	if s.Startup != nil && (s.Role != "receiver" || s.Backend != "installed" || !s.Startup.Valid() || s.Startup.Measured == 0) {
		return false
	}
	if s.Capture != nil && (s.Role != "source" || !s.Capture.Valid() || s.Capture.Measured == 0) {
		return false
	}
	for _, v := range []Vector{s.Preparation, s.First, s.Worst} {
		if v.Measured>>StageCount != 0 || (v.Outcome != "" && !validOutcome(v.Outcome)) {
			return false
		}
		for i, n := range v.Micros {
			if n < 0 || n > Window.Microseconds() || (v.Measured&(1<<i) == 0 && n != 0) {
				return false
			}
		}
		for _, b := range v.Worker {
			if b > 5 {
				return false
			}
		}
		if !ValidWorkerDetail(v.Worker, v.WorkerMillis, v.WorkerMeasured) {
			return false
		}
		if !ValidAuthorityDetail(v.AuthorityMicros, v.AuthorityMeasured) {
			return false
		}
		if !ValidPipeOwnerCycles(v.PipeOwnerCycles, v.PipeOwnerCyclesMeasured, v.AuthorityMeasured) {
			return false
		}
	}
	var total uint64
	for _, n := range s.Histogram {
		total += uint64(n)
	}
	return total == uint64(s.Frames)
}

func validOutcome(s string) bool { return s == "ok" || s == "error" || s == "incomplete" }

type Collector struct {
	mu              sync.Mutex
	emit            func(Summary)
	started         time.Time
	count           uint8
	active          [Limit]*attempt
	timer           *time.Timer
	stopped         atomic.Bool
	pipeOwnerProbes atomic.Uint32
}
type attempt struct {
	summary          Summary
	started, request time.Time
	sessionID, epoch string
	finished         bool
	firstStarted     bool
}
type session struct {
	collector        *Collector
	source, receiver *attempt
	lastClose        time.Time
}
type sample struct {
	vector         Vector
	started        time.Time
	first          bool
	workerRecorded bool
}
type sessionKey struct{}
type attemptKey struct{}
type sampleKey struct{}
type offeredKey struct{}

func New(emit func(Summary)) *Collector { return &Collector{emit: emit} }

func WithSession(ctx context.Context, c *Collector) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionKey{}, &session{collector: c})
}

func getSession(ctx context.Context) *session {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(sessionKey{}).(*session)
	return s
}

// Begin consumes a slot even when preparation later fails. The window starts
// on the first attempt, never restarts on reconnect, and expires without input.
func Begin(ctx context.Context, role, sessionID, epoch string, start time.Time) context.Context {
	s := getSession(ctx)
	if s == nil || s.collector.stopped.Load() {
		return ctx
	}
	c := s.collector
	now := time.Now()
	c.mu.Lock()
	if c.count == Limit || (!c.started.IsZero() && now.Sub(c.started) >= Window) {
		// A new untracked lease must never inherit a still-live slot from a
		// prior attempt. Freeze that role's old record before dropping it.
		old := s.receiver
		if role == "source" {
			old, s.source = s.source, nil
		} else {
			s.receiver = nil
		}
		emit := c.retireLocked(old, "incomplete")
		c.mu.Unlock()
		emit()
		return context.WithValue(ctx, attemptKey{}, (*attempt)(nil))
	}
	if c.started.IsZero() {
		c.started = now
		c.timer = time.AfterFunc(Window, c.expire)
	}
	c.count++
	if start.IsZero() {
		start = now
	}
	a := &attempt{started: start, request: now, sessionID: sessionID, epoch: epoch, summary: Summary{Schema: 1, Attempt: c.count, Role: role, Backend: "native"}}
	c.active[c.count-1] = a
	var old *attempt
	if role == "source" {
		old, s.source = s.source, a
		a.summary.Backend = "none"
	} else {
		// A request rejected for contention must not replace the active
		// receiver's profile. Activate is called after the local-state gate.
		if s.receiver == nil || s.receiver.finished {
			old, s.receiver = s.receiver, a
		}
	}
	if !s.lastClose.IsZero() {
		put(&a.summary.Preparation, ReentryGap, now.Sub(s.lastClose))
	}
	c.mu.Unlock()
	c.finish(old, "incomplete")
	return context.WithValue(ctx, attemptKey{}, a)
}

func Activate(ctx context.Context) {
	s := getSession(ctx)
	a := getAttempt(ctx)
	if s == nil || a == nil {
		return
	}
	s.collector.mu.Lock()
	old := s.receiver
	if a.finished || old == a {
		s.collector.mu.Unlock()
		return
	}
	s.receiver = a
	s.collector.mu.Unlock()
	s.collector.finish(old, "incomplete")
}

func Current(ctx context.Context, role string) context.Context {
	s := getSession(ctx)
	if s == nil || s.collector.stopped.Load() {
		return ctx
	}
	s.collector.mu.Lock()
	a := s.receiver
	if role == "source" {
		a = s.source
	}
	s.collector.mu.Unlock()
	return context.WithValue(ctx, attemptKey{}, a)
}

// Matching uses identities only in memory; a stale observer never updates a
// newer attempt. This is diagnostic association, not lease authorization.
func Matching(ctx context.Context, role, sessionID, epoch string) context.Context {
	ctx = Current(ctx, role)
	a := getAttempt(ctx)
	if a != nil && (a.sessionID != sessionID || a.epoch != epoch) {
		return context.WithValue(ctx, attemptKey{}, (*attempt)(nil))
	}
	return ctx
}

func getAttempt(ctx context.Context) *attempt {
	if ctx == nil {
		return nil
	}
	a, _ := ctx.Value(attemptKey{}).(*attempt)
	return a
}

func Record(ctx context.Context, stage Stage, elapsed time.Duration) {
	s := getSession(ctx)
	a := getAttempt(ctx)
	if s == nil || a == nil || stage >= StageCount {
		return
	}
	if v, _ := ctx.Value(sampleKey{}).(*sample); v != nil {
		// Recovery within the same dispatch must not replace the original
		// failing call with a later cleanup call's timings.
		if v.vector.Measured&(1<<stage) == 0 {
			put(&v.vector, stage, elapsed)
		}
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if !a.finished && time.Since(s.collector.started) < Window && a.summary.Preparation.Measured&(1<<stage) == 0 {
		put(&a.summary.Preparation, stage, elapsed)
	}
}

func put(v *Vector, stage Stage, elapsed time.Duration) {
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > Window {
		elapsed = Window
	}
	v.Micros[stage] = elapsed.Microseconds()
	v.Measured |= 1 << stage
}

func Backend(ctx context.Context, name string) {
	s := getSession(ctx)
	a := getAttempt(ctx)
	if s == nil || a == nil {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if !a.finished {
		a.summary.Backend = name
	}
}

func Granted(ctx context.Context) {
	a := getAttempt(ctx)
	if a != nil {
		Record(ctx, RequestToGrant, time.Since(a.request))
	}
}

func Offered(ctx context.Context, at time.Time) context.Context {
	if s := getSession(ctx); s == nil || s.collector.stopped.Load() {
		return ctx
	}
	return context.WithValue(ctx, offeredKey{}, at)
}

func Input(ctx context.Context, role string) (context.Context, func(error)) {
	ctx = Current(ctx, role)
	s := getSession(ctx)
	a := getAttempt(ctx)
	if s == nil || a == nil {
		return ctx, func(error) {}
	}
	s.collector.mu.Lock()
	if a.finished || time.Since(s.collector.started) >= Window {
		s.collector.mu.Unlock()
		return ctx, func(error) {}
	}
	v := &sample{started: time.Now(), first: !a.firstStarted}
	a.firstStarted = true
	s.collector.mu.Unlock()
	if at, ok := ctx.Value(offeredKey{}).(time.Time); ok {
		put(&v.vector, SourceQueue, v.started.Sub(at))
	}
	if v.first {
		at := v.started
		if offered, ok := ctx.Value(offeredKey{}).(time.Time); ok {
			at = offered
		}
		put(&v.vector, FirstOfferSinceBegin, at.Sub(a.started))
	}
	ctx = context.WithValue(ctx, sampleKey{}, v)
	return ctx, func(err error) {
		put(&v.vector, Dispatch, time.Since(v.started))
		v.vector.Outcome = Outcome(err)
		s.collector.mu.Lock()
		defer s.collector.mu.Unlock()
		if a.finished {
			return
		}
		if v.first {
			a.summary.First = v.vector
			return
		}
		// Saturation stops sampling, rather than wrapping or overflowing counts.
		if a.summary.Frames == ^uint32(0) {
			a.summary.Incomplete = true
			return
		}
		a.summary.Frames++
		a.summary.Histogram[bucket(time.Duration(v.vector.Micros[Dispatch])*time.Microsecond)-1]++
		if a.summary.Worst.Outcome == "" || v.vector.Micros[Dispatch] > a.summary.Worst.Micros[Dispatch] {
			a.summary.Worst = v.vector
		}
	}
}

func Worker(ctx context.Context, buckets [3]byte) {
	WorkerDetail(ctx, buckets, [3]uint16{}, 0)
}

// ValidWorkerDetail checks a fixed numeric diagnostic. Milliseconds are floored
// and saturate at MaxWorkerMillis; a zero without a presence bit is unobserved.
func ValidWorkerDetail(buckets [3]byte, millis [3]uint16, measured uint8) bool {
	if measured & ^uint8(7) != 0 {
		return false
	}
	for i, n := range millis {
		if n > MaxWorkerMillis {
			return false
		}
		if measured&(1<<i) == 0 {
			if n != 0 {
				return false
			}
			continue
		}
		if buckets[i] != byte(bucket(time.Duration(n)*time.Millisecond)) {
			return false
		}
	}
	return true
}

// WorkerDetail attaches only the matching ACK's three elapsed spans to the
// original input sample. Recovery cannot overwrite it with a later ACK.
func WorkerDetail(ctx context.Context, buckets [3]byte, millis [3]uint16, measured uint8) {
	if ctx == nil {
		return
	}
	if v, _ := ctx.Value(sampleKey{}).(*sample); v != nil && !v.workerRecorded {
		v.vector.Worker = buckets
		if ValidWorkerDetail(buckets, millis, measured) {
			v.vector.WorkerMillis, v.vector.WorkerMeasured = millis, measured
		}
		v.workerRecorded = true
	}
}
func bucket(d time.Duration) int {
	switch {
	case d < time.Millisecond:
		return 1
	case d < 10*time.Millisecond:
		return 2
	case d < 100*time.Millisecond:
		return 3
	case d < time.Second:
		return 4
	default:
		return 5
	}
}
func Outcome(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

// Retire freezes the snapshot before the session coordinator is released.
// Its returned emitter must be called outside that critical section.
func Retire(ctx context.Context, outcome string) func() {
	s := getSession(ctx)
	a := getAttempt(ctx)
	if s == nil || a == nil {
		return func() {}
	}
	s.collector.mu.Lock()
	if !a.finished && (s.source == a || s.receiver == a) {
		s.lastClose = time.Now()
	}
	emit := s.collector.retireLocked(a, outcome)
	s.collector.mu.Unlock()
	return emit
}
func Finish(ctx context.Context, outcome string) { Retire(ctx, outcome)() }

func EndSession(ctx context.Context) {
	for _, role := range []string{"source", "receiver"} {
		Finish(Current(ctx, role), "incomplete")
	}
}
func (c *Collector) finish(a *attempt, outcome string) {
	c.mu.Lock()
	emit := c.retireLocked(a, outcome)
	c.mu.Unlock()
	emit()
}
func (c *Collector) retireLocked(a *attempt, outcome string) func() {
	if a == nil || a.finished {
		return func() {}
	}
	a.finished = true
	a.summary.End = outcome
	a.summary.Preparation.Outcome = outcome
	a.summary.Incomplete = a.summary.Incomplete || outcome == "incomplete" || a.summary.First.Outcome == ""
	summary := a.summary
	c.active[summary.Attempt-1] = nil
	return func() {
		if c.emit != nil {
			defer func() { _ = recover() }()
			c.emit(summary)
		}
	}
}

func (c *Collector) expire() {
	c.stopped.Store(true)
	c.mu.Lock()
	active := c.active
	c.mu.Unlock()
	for _, a := range active {
		c.finish(a, "incomplete")
	}
}
