# g538 paired crossings: lower A activation tails, retained reconnect and B stall

Collected fresh diagnostics from both notebooks after the user completed the
trial, collection `72c9b868099934d3e5a23e2199cd10cc`, exact installed revision
`59f58658c1f35bb2aa15170bb81000072e0a0ba1`. CLI doctor is healthy/CONNECTED;
both installed generations/builds agree. The adjacent JSON records sanitized
evidence hashes, selection, retained incidents and matched samples.

## Activation and focus

The latest authenticated connection has 21 matched A-source/B-receiver profiles,
20 complete OK pairs and one pair marked incomplete by the bounded collector.
That incomplete label is not itself evidence of a failed lease or reconnect.
Its available timings are reported separately instead of silently discarded.

| Local measured interval | g537: 14 complete, median/max ms | g538: 20 complete, median/max ms | g538: all 21 measured, median/max ms |
| --- | ---: | ---: | ---: |
| A capture activation | 18.888 / 236.627 | 13.390 / 61.325 | 13.406 / 73.523 |
| A foreground request/exchange | 11.610 / 95.860 | 7.785 / 36.725 | 7.807 / 56.859 |
| A request to grant | 38.619 / 63.960 | 43.969 / 213.021 | 45.345 / 213.021 |
| B worker readiness wait | 24.114 / 25.766 | 26.592 / 81.909 | 27.608 / 81.909 |
| B first input dispatch | 1.855 / 4.100 | 2.292 / 6.739 | 2.350 / 6.739 |
| B worst input dispatch per entry | 4.621 / 144.752 | 19.750 / 393.157 | 19.852 / 393.157 |

A activation/focus tails are lower in this sample, even including the incomplete
pair: observed maxima declined by 68.9% and 40.7%. Workload, sample counts and
observation periods differ, so this is not a controlled causal speedup estimate.
Other stages regressed; an overall smoothness improvement is not established.

The slowest complete A activation (61.325ms) includes worker activation 52.381ms,
foreground request 36.725ms and caller resumption 7.572ms. Its request delivery
was 0.166ms and read authorization 0.257ms. The foreground request is nested in
worker activation, not an additional interval. Foreground verification is
measured zero in all 21 profiles; omitzero/omitempty fields must not be treated
as missing measurements when their measured bits are set.

## Reconnection and service recovery

One earlier g538 disconnect/reconnect incident is retained. On A's clock it began
at **2026-11-15 04:04:18 KST**: `installed_source_reader_retired`, then the
source-hook trigger `fault_source_hook_failed` and DISCONNECTING. B recorded
`fault_remote_eof`, RECONNECTING and two reconnect acquisition timeouts on the
same old connection. Both reached the same new connection, each about 12.6s
after its locally recorded trigger. The clocks differ; do not subtract A times
from B times to determine causal order or transport latency.

The reader-retired label specifically means retired=true with nil reader error
in `sourceFailureReason`. A's matching service exit has command/report OK,
capture/sender/interrupt joined and clean_sent retirement. This supports a
targeted follow-up on retired-worker handling, not a claim that heartbeat reply
starvation or uncertain native cleanup recurred. The immediate reason for the
retirement/fault classification is not established by these records alone.

Both service start times are unchanged since installation, service/input health
are READY and grants PRESENT. The recovery ledgers retain their pre-g538 last
failure timestamps. There is no evidence here of a new error-1066 service stop
or an administrator-required service repair.

Since the latest connection, A retains 1,827.256s and B 1,829.537s (about 30.5min)
with no new fault, disconnect, reconnect or recovery-required event. This does
not erase the earlier incident. Rotated agent logs cover only roughly the last
1h49m/1h39m; historical control.previous logs are incomplete. The entire nine-hour
installation period cannot be certified free of other incidents.

## Remaining latency

B's worst dispatch is 393.157ms. The matching native-child IPC includes child
handler 26.968ms (SendInput submission 23.335ms nested inside it), and
**337.190ms from reply-ready to the parent's first read return**. This latter
span includes writing, pipe delivery and receiver scheduling/resumption; it does
not isolate a slow kernel pipe write or prove a network delay. The outer service
apply interval is 379.701ms. Nested spans must not be added together.

A's grant wait reaches 213.021ms; B's matching readiness wait is 81.909ms.
These overlapping observations do not isolate grant network transit from the
remaining peer work. Sampled TCP RTT is median 5.997ms, maximum 8.240ms.
A's sampler lateness reaches 2,533.549ms in a separate runtime sample, not a
matching explanation of the 393.157ms B dispatch.

## Verification and scope

Verified live doctor/build agreement and paired READY/PRESENT state, support
hashes, duplicate-event removal, exchange-tag uniqueness, all 21 pair matches,
frame-histogram sums, measured-zero handling, unchanged service starts and the
earlier-versus-current connection incident split. No new runtime tests, build,
reinstall, remote CI or push: this request is diagnostic review only. Only these
experiment reports change; no source-of-truth contract, API, DB, dependency,
repository hygiene rule or CI runner changes.
