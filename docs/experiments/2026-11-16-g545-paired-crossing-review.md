# g545 paired crossing review

## Scope and evidence

The user confirmed natural cursor-height continuity in both directions, then
requested fresh paired diagnostics for residual latency and failure recurrence.
No runtime, timeout, protocol, API, database, runner or deployment change is made
by this review. Both notebooks remain on g545 revision
`77a66db51d5d2c87f48e3f143d98ea053065d705`.

Collection `157ad7e500be6c4189724ae0dfdb151d` completed with RECEIVED.
A was collected at 2026-11-15 18:20:17 UTC (November 16, 03:20:17 KST),
B at 18:20:07 UTC. The companion JSON retains input hashes, per-device
provenance and the eleven matched exchange tags. Pairing uses tags, never
opposite-PC wall-clock subtraction. Repeated log entries are deduplicated by
run, sequence and code. Current-build events are separated from historical logs.

## Connection and service health

- CLI doctor is healthy/CONNECTED; CLI, agent, UI, launch record and installed
  agent match g545. Peer status is ONLINE and transfer status is IDLE.
- Both input-health and service states are READY. The service start timestamps
  match the post-install collection `3e65097e7d35ef3494f4db93b7079e37`.
- One common connection is retained on both PCs, for approximately 27m19s on
  each local clock. No post-connect reconnect, timeout, fault, worker poison,
  failed session end or recovery-required event appears in the exported logs.
- B has eight acquisition network timeouts and one RECONNECTING transition
  before the common connection was established, during staggered installation.
  These are not recurrent disconnects during the crossing trial.
- A's last stored agent fault predates g545 service startup; B's stored fault
  dates to November 4. Neither is a new recovery failure in this trial.

## Crossing timings

Eleven source/receiver profiles match, all complete with end=ok. There are no
unmatched or incomplete profiles. B reports using prepared workers in 11/11.
Values below are local software durations, in milliseconds.

| Operation | Median | Maximum |
| --- | ---: | ---: |
| A boundary to beginning entry | 2.216 | 5.652 |
| A request to matching grant | 39.552 | 61.972 |
| A capture activation | 20.527 | 47.237 |
| B authorization total (includes readiness) | 29.454 | 49.463 |
| B input readiness response wait | 25.395 | 36.337 |
| B initial cursor preparation | 3.573 | 4.223 |
| B first input dispatch | 1.880 | 2.866 |
| B per-lease worst input dispatch | 2.952 | 7.126 |
| B native handler in that dispatch sample | 1.743 | 4.052 |
| B SendInput in that dispatch sample | 1.089 | 2.901 |
| A capture deactivation on natural return | 10.644 | 13.494 |

The natural B-to-A return is a lease revoke, not a fresh B-source lease.
The deactivation measurement covers only A's local restore/deactivation work;
it is not a full return input-to-display measurement. User dwell time is not
counted as system latency. Nested spans and independent maxima are not additive.

## Residual waits

The largest retained entry wait is A's request/grant span, with B readiness
still taking 23.232–36.337ms even when prepared workers are used. Socket RTT
estimates across the 22 observations are 4.557–8.242ms (median 5.264ms).
All eleven query pairs have known counters with zero retransmitted-byte and
timeout-episode increments. This supports prioritizing processing waits in
these samples; RTT estimates are not application RTTs and must not be subtracted
from request/grant timings to derive processing time.

A's slowest capture is 47.237ms. In that same exchange the child activation
is 45.918ms and the exact foreground request is 39.489ms. Request delivery is
0.111ms, read authorization 0.240ms, dispatch 0.062ms and caller resume 0.050ms.
These nested measurements locate the observed peak in native activation/focus,
rather than a long request-pipe or app-resume wait. Across all eleven captures,
foreground request median/max is 11.865/39.489ms; request delivery max is 0.111ms
and caller resume max is 2.021ms. Measured omitted scalar fields mean zero,
not missing observations; capture queue-to-handler max is 0.549ms.

A also records a main-loop timer-lateness maximum of 475.217ms and a diagnostic
sampler-lateness maximum of 1196.452ms. The sampler maximum was at 17:57:31 UTC,
before the retained crossing profiles (18:10:32–18:10:56 UTC); that summary
contains zero input dispatches. Its main-loop timer maximum is separately
69.358ms. Therefore the 1.196s sampler delay is not evidence of a 1.196s cursor
freeze or an identified CPU/GC cause. B sampler-lateness max is 10.594ms.

## Validation and limits

- Paired collection, doctor, peer build/status, matching tags, measured masks,
  unchanged service starts and input readiness checked successfully.
- Assertions check eleven complete matched profiles and zero post-connect alarm
  records; raw input hashes are retained for reproducibility.
- No source tests, new package, installation or CI run: runtime code is unchanged.
- Retained profiles are bounded samples and end before collection. The agent
  current/previous logs are complete; `control.previous.jsonl` is incomplete on
  both PCs. Absence in these exports cannot prove long-duration absence.
- No physical-input-to-display or controlled before/after experiment was run.
  g545 fixes height forwarding, so this review does not claim a speed improvement.
