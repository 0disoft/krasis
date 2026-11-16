# g548 paired crossing review

The user completed the idle/fast crossing trial and still perceived stutter.
Collection `057a0b9fffa601a12441bafb6b5bfa29` contains twelve uniquely matched
g548 source/receiver exchanges on revision
`0d5553df9a661fb0784b697082177a6da74af3af`, with no unmatched exchanges. All
ended `ok`; ten have complete input measurements. Two receiver profiles have
zero input frames and `incomplete: true`; their measured preparation stages are
retained, but they are not counted as complete input samples. The JSON companion
records matching tags, field presence, input hashes and exact local durations.

Both devices remain READY on the same connection. A's observation window is
168.943s and B's is 170.762s. No post-connect timeout/fault/poison/reconnect or
recovery-required event appears in the retained g548 run. A doctor is healthy
and CONNECTED. Current/previous agent logs are complete; control.previous is
incomplete. This is a bounded observation, not a long-term reliability claim.

## Measured local timings

Values in ms, median / maximum. g547 n=11; g548 preparation n=12, input n=10.
The workloads and system load were uncontrolled, and g547 A was traced. These
are measured software spans, not input-to-display latency or a controlled
speedup. Nested startup/native spans can include earlier prewarming.

| Operation | g547 | g548 |
| --- | ---: | ---: |
| B readiness response wait | 23.348 / 582.901 | 22.702 / 28.489 |
| A request to grant | 44.730 / 612.465 | 47.991 / 131.379 |
| A capture activation | 19.508 / 25.621 | 36.882 / 68.816 |
| A natural-return deactivation | 11.500 / 22.206 | 19.416 / 61.779 |
| B worst retained per-lease input dispatch | 4.629 / 40.086 | 3.643 / 6.846 |

All twelve B readiness profiles contain the new admission sub-stages. Gate wait
is zero at microsecond resolution in every sample. Fresh console observation
is 4.479--6.679ms (median 4.964ms), equal to total admission at this resolution.
Remaining authenticated native-pair binding/readiness is 9.265--11.990ms.
The old 582.901ms readiness tail was not reproduced. This does not establish
that diagnostic persistence caused every part of that historical tail.

## Remaining stutter

The 131.379ms grant sample has matching tag
`5bd4686cc805c10c43b7f99def290cbfd5fdab9bde3be27ca994f1e4f41e421f`.
A's successful request-write-to-grant-read interval is 130.869ms, while B's
request-read-to-grant-written interval is 30.521ms: residual 100.348ms. B
readiness itself is 22.356ms. A grant-read-to-handler is zero at measured
microsecond resolution. RTT estimates are 7.579/17.871ms, with zero new
retransmitted bytes or timeout episodes. Nine socket probes found no queued
bytes, but the largest probe gap is 27.606ms. These observations do not split
network transit, OS completion, TLS-buffer residency or A reader resumption;
the 100.348ms residual must not be labeled pure network RTT or proven A stall.
Independent A/B wall timestamps are never subtracted.

The 68.816ms A capture sample has tag
`f5232cf833937ec91f51f010ea8a0f89ccdff0da5d7f37f6276860564ed2f844`.
Its existing capture IPC diagnostics report native activation 14.694ms, reply
delivery 10.967ms and caller execution-resume 41.354ms. Foreground request
7.368ms is nested inside activation, not an extra contribution. Request
delivery is 0.062ms and authority wait is zero. This concretely locates a
remaining A execution-resume tail, separately from the unresolved grant-wait
residual. That receiver has no retained input samples, so successful handoff
termination is not evidence of successful ordinary input for that exchange.

B preparation is no longer the dominant measured tail in this trial. A capture
and return timings are higher than the previous uncontrolled trial, consistent
with the user's remaining stutter. This review makes no additional runtime
change and does not claim the entire user-visible stutter resolved.

Validation: exact revision/connection identity, measured-field presence, twelve
unique matched exchanges, two incomplete-input flags, error/log coverage,
causal local-duration arithmetic and JSON/report consistency were checked.
No source tests, remote CI or Linux VM test was repeated for this evidence-only
review. No API, DB, dependencies, repository hygiene or runner changes.
