# g537: keep active input backpressure outside the source reply gate

## Evidence and diagnosis

Paired g536 collection `080d1da1e10bc93d73f9cce011c8e7e0` retains A's
2026-11-14 08:57:29Z terminal source-hook failure, heartbeat `reply_gate`
slow bucket >1s, and sampled holder `source-gate-action-write`. Both protected
services were READY at inspection; the active session was FAULTED. The holder
sample cannot establish that this action owned the whole wait. Process-wide
scheduling delays and earlier independent authority-query failures also remain
possible contributors.

Code inspection found active source actions could fill the 4KiB worker-to-agent
pipe with no consumption window. Boundary observations already had a one-batch
window. An additional action then waits in WriteFile while holding the exact
gate required by heartbeat replies and joined-retirement notification. Merely
prioritizing gate waiters cannot free a pipe write already in progress.

## Change

Bound unconsumed actions to 16 x 64 bytes (1KiB), alongside the independent
1KiB boundary window. Return capacity after every eight consumed actions, via
the existing authenticated one-way private receipt command. A fixed three-slot
receipt queue holds one boundary and two action-group receipts. Exact ending
sequences reject duplicates, future or partial-group references. Group counting
continues across epochs, including intentionally ignored old-epoch frames.

Wait for capacity outside serialization, proof queries and the existing
one-second write budget. Both owner and caller cancellation stop this wait.
Afterward, fresh authority and generation checks remain mandatory. Normal input
credit acquisition adds no timer/context/callback. Keep the existing SourceSession
queue/coalescing, action order and attempted-write failure/poisoning policies.
Receipts certify pipe consumption only: they do not renew liveness, grant input,
claim remote delivery or replace independent native-cleanup evidence.

The source worker and medium agent remain the exact same pinned build; no public
API, peer wire format, DB, dependency, workflow or runner change is required.
The existing full installer/peer update path preserves prior binaries/settings.
Do not resume an earlier revision through mixed worker/agent images.

## Validation

New tests cover real Windows 4KiB pipes: pause consumption for 1.1 seconds with
16 boundary and 16 action frames queued, require heartbeat and retirement writes
before any reads, and verify all original ordered frames. Further tests exercise
128 inputs/16 receipt round trips across ten epochs, cancellation, stale-generation
rejection, exact group boundaries, duplicate/future receipts and ring wrap/rollback.

These tests establish bounded pipe occupancy and gate-independent capacity waits,
not elimination of arbitrary CPU pauses or physical latency improvement. Paired
physical crossings and a long soak remain separate acceptance work.

Passed locally with the repository's verified runtime overlay:

- `go test ./internal/inputworker ./internal/wininput ./internal/residentagent
  -skip TestPortablePair -count=1 -timeout=120s -p=1` (27.126s / 3.274s / 7.674s).
  Only the fixed-owner portable-pair fixture was excluded to avoid the installed
  resident service; native pipe tests were executed.
- `go test -race ./internal/inputworker -run 'Test(SourceAction|NativeAction)'
  -count=1 -timeout=90s` (2.373s; repository-local LLVM/CGO toolchain).
- `go test ./internal/localapi ./internal/productcli -count=1 -timeout=90s -p=1`
  (7.241s / 5.467s).
- `go vet -unsafeptr=false ./internal/inputworker`; `git diff --check`.

Linux VM, hosted CI and signed public release validation are not part of this
Windows private-pipe change. Packaging and installation are checked separately.
