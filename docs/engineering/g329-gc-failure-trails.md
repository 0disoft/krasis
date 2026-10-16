# GC observations around connection failure

The g327 A snapshot in collection `c9854838a235a576b9e16157af307409`
contains a GC pause in the highest aggregate bucket, approximately 1.62s timer
lateness and a source heartbeat write failure in the same final 4.29s interval.
This is a candidate relationship, not proof of order or cause. The six bucket
groups conservatively classify runtime histogram upper bounds; their labels do
not provide an exact pause duration. G328 later records another source failure
with a highest-bucket GC pause and approximately 3.55s timer lateness.

## Bounded agent observation

The resident agent samples runtime metrics off input, reader and heartbeat paths
every 100ms into a fixed 64-window ring. It records independent delta histograms
for total GC stop-the-world latency, time spent stopping all Ps, and runnable
scheduler delay. It also records allocation/cycle counters, current heap size,
estimated GC assist/total CPU, sampler read duration and ticker lateness.
Unknown or reset counters have cleared availability bits, not measured zeros.

Only the first termination trigger for each connection requests a trail.
Explicit cancellation can also trigger termination; match TriggerObservedAt to
the session-end record and its canceled/timeout flags before calling it a fault.
The callback submits scalar values to a one-slot nonblocking channel; no runtime
query, JSON/disk work or wait is added there. A pending request is drained during
the sampler's explicit shutdown, after the owning runner joins its callbacks.
At most eight trails are logged per owning runner. Coalesced/limited triggers
are counted and a fixed limit event makes omitted evidence visible. Normal
100ms samples are memory-only; existing minute runtime summaries remain intact.

Each trail includes monotonic interval boundaries and callback notification time,
plus the original failure observation's wall time and allowlisted fault code.
A window crossing or following notification must not be labelled wholly before
the failure. These are observation windows, not exact GC start/end timestamps;
timers may themselves run late during a pause. Total/stopping bucket maxima must
not be subtracted to infer GC work duration, and their deltas may cover different
completed pauses. Runtime CPU estimates are not OS CPU measurements.

The sampler covers the resident agent process, not the separate privileged
service/source worker or WebView. It does not change GOGC, GOMEMLIMIT, liveness
timeouts, lease rules, input retries or native retirement requirements.

## Export and verification

`runtime_gc_trail` exports only fixed numeric windows and validated timing/fault
fields. Maximum-ring records require a bounded 64 KiB scanner line; every other
record retains its original 4 KiB limit and the total file limit is unchanged.
Unknown nested fields are stripped. Invalid masks, windows, chronology and
unbounded sample counts are rejected. No raw execution trace, stack, input
content, credentials, window names or arbitrary paths are collected.

Tests cover schema export/privacy, maximum-size records, detached queued data,
counter resets, independent histograms, ring wrap/order, post-failure windows,
nonblocking allocation-free callbacks and failure/shutdown ordering. The local
sampler benchmark measured about 4.46us/read with 0 B/op and 0 allocs/op on this
host; that is not a real-device latency or Windows scheduling guarantee.

Go metric semantics: https://go.dev/src/runtime/metrics/description.go
GC latency background: https://go.dev/doc/gc-guide#Latency

No public API, peer wire, database, dependency, ACL or runner change.
