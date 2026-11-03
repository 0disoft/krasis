# g523: bounded reusable native proof dispatch

Baseline: g522 collection `3fa56959eb86b67e539a62746ae2470c`, ten complete
matched A-source/B-receiver leases. B's worst dispatch was 59.334ms:

| Containing phase | Duration |
| --- | ---: |
| Service admission | 9.653ms |
| Service's native child exchange | 48.716ms |
| Native request-to-child-read | 11.406ms |
| Native child handler | 33.173ms |
| SendInput, nested inside handler | 6.781ms |

These are nested scopes, not quantities to sum. Handler time includes fresh
service/agent/desktop validation and scheduling, not just SendInput. The native
timings relayed in this sample were 9ms service proof and 7ms desktop scope;
rounding, overlapping proof spans and scheduling prevent subtracting these to
invent a precise remainder. Request-to-read includes parent Write, kernel
delivery and child execution/resumption. It is not evidence of network latency.

## Change

The native input child previously created a service-check goroutine and reply
channel for every framed command. The child now owns one reusable proof worker,
one buffered request slot and one reply slot. Atomic admission permits only one
outstanding proof. Normal request admission allocates neither a reply channel
nor a new goroutine. A finish receipt permits slot reuse only after the exact
result was consumed. An unjoined timeout permanently fences reuse, including
after late success. Busy admission fails closed rather than accumulating work.

Every admitted command still performs its own fresh service query. Desktop
validation overlaps it as before, and a late proof still requires a fresh
desktop/agent check before input. No input callback runs on the proof worker.
Shutdown joins an in-flight native query before closing service/agent handles;
parent job retirement and uncertain-input no-replay rules are unchanged.
Panic and Goexit deny input. In particular, Goexit must not publish the default
nil error from an abandoned proof callback.

A synthetic 20,000-call dispatcher benchmark on the packaged runtime recorded 160 B/op and 3 allocs/op
for spawning versus 0 B/op and 0 allocs/op for reuse. Observed time was about
2.21us versus 2.09us; this tiny result is not a Windows input latency claim.
At 1000 commands/s this removes about 3000 temporary allocations and 1000
goroutine launches per second. Actual latency needs the physical trial.

## Request delivery diagnostic

The existing bounded native IPC profile now includes optional
`requestWriteMicros` and its presence bit. One extra monotonic read after a
successful Write measures the parent's synchronous write duration. There is
no additional IPC exchange or byte: the 68-byte local diagnostic suffix uses
reserved header bytes, normalized variant 5 and an outer variant-4 presence bit.
Legacy variants remain accepted; malformed/reserved metadata is discarded.
Only a matching successful response can publish the measurement.

The value is nested in the full exchange. Parent write completion can follow
the child's read when the parent resumes late, so it must not be blindly
subtracted from request-to-read or added to other phases. Clock regressions,
missing timestamps and values beyond the matching envelope are unobserved.
The diagnostic can distinguish a short parent write from a long delivery/read
interval; it does not by itself identify kernel arrival, CPU starvation or pipe
contention. Input/control protocol, sequencing and replay behavior are unchanged.

## Validation and boundaries

Tests cover fresh queries and revocation, bounded admission, cancelled work,
pending proof fencing, late completion, shutdown join, panic/Goexit, diagnostic
roundtrips with legacy and new variants, write/read overlap, exact bounds and
invalid clocks/reserved bytes. Package/race/vet/build and paired installation
results are recorded separately. No claim that 33.173ms or 11.406ms is eliminated
without the new physical trial. No push or remote CI is part of this task.
