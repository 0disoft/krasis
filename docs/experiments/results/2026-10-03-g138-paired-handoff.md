# g138 paired handoff latency evidence

## Scope and selection

The operator exported both device diagnostics after the requested crossing
trial. A then invoked `krasis-cli --json diagnostics collect B`; collection
completed as `saved`/`RECEIVED` without disconnecting either device. The fresh
exports were created at `2026-10-03T13:33:21Z` (A) and `13:33:22Z` (B).

Generation is 138, revision `c3566d86d2066e8645fe567354966ce4bb356aa7`.
The [sanitized evidence](../../reviews/g138-paired-handoff-evidence.json)
preserves the first six profiles of each device's newest `agent_ready` run,
plus each local runtime interval overlapping those profiles. All selected
records match this build. A started at `13:18:56.6531713Z`; B started at
`13:26:09.594408Z`. Both collectors were therefore fresh. A later seventh
profile on each device is excluded from the six-crossing analysis.

Each device recorded three source and three installed receiver profiles.
All twelve profiles have `end:ok`, `incomplete:false`, and histograms whose
sums equal their measured steady-state frame counts. The fixed 39-stage
vectors, duration bounds, presence masks and worker bucket bounds were checked.
Device-local attempt ordinals and opposite-host timestamps are not treated
as shared lease identifiers. No physical input or displayed latency was
independently reproduced by this agent.

## Observed preparation and steady-state processing

| Local measurement | A source / B receiver | B source / A receiver |
|---|---:|---:|
| Source request-to-grant, three entries | 454.837 / 412.187 / 464.454 ms | 200.401 / 192.695 / 169.926 ms |
| Receiver ready wait, three entries | 402.337 / 362.206 / 397.344 ms | 158.323 / 155.979 / 139.120 ms |
| Receiver service authentication | 30.444 / 31.666 / 38.513 ms | 9.032 / 7.763 / 8.743 ms |
| Receiver maximum steady dispatch per profile | 10.623 / 2.888 / 9.344 ms | 2.187 / 4.168 / 1.848 ms |

Rows overlap or describe different operations; they must not be summed.
The receiver's ready wait remains the clearest repeated long preparation
span. The previous 15.186 ms client-authority worst-frame observation is
not repeated in these selected worst vectors: their client-authority spans
are at most 0.756 ms. Independent timing-window authority maxima are below
10 ms in this run, but those records are not individual input counts.

Across the selected source and receiver steady-state histograms there are
no dispatch samples at or above 100 ms. This is bounded software evidence,
not proof that the user experienced no movement stutter. Source first-frame
coordinator spans reached 22.707 ms on A and 34.073 ms on B; source capture
activation reached 23.250 ms and 37.890 ms respectively. They overlap and
do not explain the receiver's several-hundred-millisecond ready wait.

The purported quick reentries have local gaps of 917.678/931.822 ms
(A source/B receiver) and 1011.635/1037.361 ms (B source/A receiver).
Delayed reentries have gaps of approximately 2.3-2.5 seconds. This trial
does not exercise a sub-200-ms reentry or establish that the service's
200-ms accept backoff is responsible. Intended four-second movement
durations and subjective latency observations have not been confirmed.

## Runtime and completeness

| Trial-overlapping runtime interval | A | B |
|---|---:|---:|
| Queue maximum residence | 16.181 ms | 25.694 ms |
| Queue high watermark | 1 | 1 |
| Coalesced moves | 1 | 3 |
| Stale discards | 0 | 0 |
| Hook maximum | 1.951 ms | 1.014 ms |

These are interval summaries, not the queue spans of the selected worst
frames. Runtime `dispatchMaxMicros` includes non-input dispatches and must
not be equated to steady pointer latency.

Both current and previous agent logs are complete in these exports. Each
export also flags an empty previous control log as incomplete; it does not
make the inspected agent profiles incomplete, but prevents a claim that all
historical UI evidence was captured. No failure/reconnect/drop/unknown codes
were found in the selected agent runs. This does not independently verify
PIN cancellation or every physical input outcome. Current A status was
`CONNECTED` to B with `inputBackend:PROTECTED`.

## Next bounded investigation

The next instrumentation target is the receiver ready path: worker launch,
child authentication and input-desktop readiness. Measure these substeps
within the same attempt before choosing a performance patch. Do not start
with worker reuse, skipped authority checks, image-verification removal,
ACK removal or a shorter accept backoff. Preserve the already working
protected-input path and require a separate physical PIN regression check.

This result changes documentation and sanitized evidence only. No executable,
generation, input behavior, service, installation, security configuration,
API, database or runner was changed. JSON/schema/privacy checks and
`git diff --check` cover this evidence-only change; application builds,
unit tests and repeated physical tests are intentionally not performed.
