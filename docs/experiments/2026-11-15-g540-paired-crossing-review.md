# g540 paired trial: A activation still slow, one source-query retirement

The user reported completed physical crossings with stutter. Paired collection
`6f4299ce90ae2d397ca8c3f2f92c3bb1` captures exact g540 revision
`f2dfd5d4552d65e2715ddadc4694a449277b2fb6`. Live doctor is healthy/CONNECTED;
CLI, agent, UI, launch record and protected installation agree. B is ONLINE on
the same generation/revision with transfer IDLE. The adjacent JSON preserves
sanitized evidence hashes, matched measurements and retained incidents.

## Timing result

All 10 retained A-source/B-receiver profiles match by exchange tag and are
complete/OK, covering 1,933 receiver frames. The A profile timestamps range from
11:22:23 to 11:38:27 UTC. These are all retained profiles since installation,
not a manually isolated subset of the latest motion. No profiles are dropped
because they occur before or after worker replacement.

| Local measured interval | g539 median / max ms, 13 pairs | g540 median / max ms, 10 pairs |
| --- | ---: | ---: |
| A capture activation | 12.888 / 67.755 | 51.917 / 135.798 |
| A foreground request/exchange | 7.106 / 47.028 | 33.817 / 57.962 |
| A post to capture-owner handling | 0 / 1.017 | 1.149 / 17.928 |
| A request to grant | 41.514 / 45.612 | 47.914 / 94.111 |
| B worker readiness wait | 23.909 / 25.233 | 24.695 / 27.862 |
| B first input dispatch | 1.966 / 3.574 | 2.271 / 8.523 |
| B worst input dispatch per entry | 5.229 / 44.978 | 3.471 / 71.448 |

There is no measured A latency improvement. The g540 optimization's deterministic
native-call reduction does not establish a physical speedup. Observed A medians
and maxima regressed, but different workloads, sample counts and system pressure
do not establish that the source change caused this regression.

## Where the slow intervals occur

The worst A activation is 135.798ms. Its same-command worker activation is
116.075ms, containing post-to-owner handling 17.550ms and foreground exchange
57.962ms. Earlier worker read authorization is 7.588ms and dispatch 5.966ms;
reply admission is 2.683ms. Pipe request delivery is 0.117ms, reply delivery
0.068ms and caller resumption 0.604ms. The primary observed tail is therefore
inside activation/owner work, not reply transport. Foreground exchange still
mixes monitor scheduling, desktop validation, the native API and owner pumping;
the remaining native setup intervals only have buckets. The complete 116ms
cannot be assigned to one Windows call with this metadata.

The same entry's first-offer-since-begin is 383.086ms. This includes source input
availability and must not be described as cursor latency or a network RTT.
Sampled TCP RTT across the trial is median 14.508ms, max 37.307ms; no retained
sample reports retransmitted bytes or timeout episodes. Nested spans cannot be
added, and A/B wall clocks cannot be subtracted to infer network transit.

B's worst dispatch, 71.448ms, has native child handler 39.969ms (SendInput
submission 14.249ms nested inside), request delivery 10.708ms and
reply-ready-to-first-read 10.429ms. Maximum reply-to-read across the retained
worst-input frames is 14.090ms in a different entry. The old 337ms reply tail
did not recur in these retained frames, but B still contributes occasional tails.

A's sampler lateness reaches 1,100.749ms in the minute covering the source
retirement, and 359.119ms in the minute containing the slowest activation.
Earlier bounded OS snapshots show substantial system CPU pressure, with
per-trail weighted system busy ratios between 48.5% and 99.6%. Those snapshots
end at 11:27:36 UTC, before the later retirement and worst activation; they
support investigation of scheduling pressure, not causal attribution of either
incident. No offending process or specific GC pause is established here.

## Failure and recovery classification

One real source-worker failure occurred on A at 11:34:56 UTC (20:34:56 KST).
The service exit report identifies `source-service-query-deadline-scheduled`,
reader authority rejection and heartbeat service_presence delay. In
channel_authority.go the query changes scheduled to checking at callback entry;
the report therefore identifies deadline expiry before this service-query
callback began its check. It does not prove a slow SCM call or a denied service.
The missing failure-stage native timings cannot be treated as a successful query.

The same incident includes heartbeat response failure and a cancelled boundary
ACK. Those labels are not eleven independent incidents. Capture, sender and
interrupt owners all joined, retirement was clean_sent, but the authority
report is failed. The runtime recorded refresh_joined at 11:34:56.9092794 UTC,
then protected capture READY at 11:34:58.6951063 UTC, about 1.786s later.
Seven complete crossing profiles occur after this replacement.

No new DISCONNECTING, RECONNECTING, session-stopped or TLS session fault is
retained after the installation connection. A and B retain the same shared
authenticated connection for 1,027.151s / 1,028.420s, roughly 17 minutes.
Both services/input are READY with grants PRESENT. Service start times and
recovery last-failure ledgers are unchanged, so no new error-1066 service stop
or administrator-required repair is evidenced. This is a worker replacement
without transport reconnect; it must not be reported as no source failure.

## Verification and next target

Verified evidence hashes, exact builds, event deduplication, unique matching,
frame-histogram sums, measured-zero handling, source-query phase semantics,
retirement-to-ready ordering, unchanged service starts/recovery ledgers and
incident versus timing-label counting. Source tests, rebuild, reinstall, remote
CI and push were skipped because this step records diagnostic evidence only.
No API, DB, dependency, hygiene, runner or source-of-truth contract changes.

The next investigation should separate source service-query scheduling before
callback entry from native query time, and foreground monitor/API work from
capture-owner queue resumption. Lengthening deadlines or removing fresh proof
checks would not resolve the demonstrated scheduling gap. The current data is
insufficient to justify another speculative Win32 activation change.
