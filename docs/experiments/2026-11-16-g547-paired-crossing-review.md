# g547 physical crossing and receive-resume review

The user completed the requested idle/fast crossings and reported no perceived
issue. Collection `3999ecba3bc4b30c6ec425a43cda2d6b` retains eleven uniquely
matched source/receiver exchanges on g547 revision
`7671397cf4f786f1ed873e65c886a165710f55cb`. All eleven ended successfully;
none is incomplete or unmatched. The JSON companion records input hashes,
numeric observations, matching tags, measured fields and trace evidence.

## Health and observation scope

Both inputs are READY, and both devices retain the same connection. A's
post-connect observation is 458.557 s; B's is 459.379 s. No post-connect
timeout, fault, worker poison, session failure, recovery-required event or
RECONNECTING event appears in the retained g547 runs. B's earlier acquisition
timeout belongs to staggered installation, before the final connection.
Stored recovery lastFailure entries are older failures, not new g547 events.
Agent current/previous logs are complete; control.previous is incomplete.
This is a bounded observation, not an indefinite reliability guarantee.

## g546 versus g547

Local software timings in ms; g546 n=8, g547 n=11. Trials differ in workload
and host/network conditions; A was traced in g547. The table is an observation,
not a controlled speedup or physical input-to-display measurement. Nested
spans and separate maxima must not be summed.

| Operation | g546 median / max | g547 median / max |
| --- | ---: | ---: |
| A boundary to entry begin | 5.050 / 29.925 | 1.659 / 2.926 |
| A request to grant | 49.016 / 325.072 | 44.730 / 612.465 |
| A capture activation | 34.924 / 60.450 | 19.508 / 25.621 |
| A natural-return deactivation | 11.500 / 22.206 | 7.685 / 16.216 |
| B readiness response wait | 24.266 / 119.843 | 23.348 / 582.901 |
| B first retained input dispatch | 1.813 / 2.312 | 2.313 / 3.312 |
| B per-lease worst retained input dispatch | 3.427 / 4.678 | 4.629 / 40.086 |
| B SendInput in worst-dispatch samples | 0.591 / 1.733 | 0.654 / 15.780 |
| B service reply transfer/app resume in those samples | 0.552 / 1.752 | 1.758 / 38.716 |

Ten of eleven grant waits are 38.653--63.732 ms. The remaining 612.465 ms
sample must not be removed when reporting the tail despite the user's smooth
experience. A's capture values are lower in this trial, but the overall grant
tail is higher and B retains input-response tails.

## The 612 ms exchange

Matching tag:
`f8d18a6a708faf9900273e0de0474ea7d01950cd1d9d9a251be9c3e36d17a169`.

* A request-to-grant: 612.465 ms; successful request-write-to-grant-read:
  611.914 ms; request write: 0.550 ms.
* B request-read-to-grant-written: 599.315 ms; authorization: 589.256 ms;
  readiness response wait: 582.901 ms; entry preparation: 8.269 ms.
* The difference between the two causal local exchange durations is 12.599 ms.
  It includes transport, OS delivery and unmeasured intervals, not pure
  network RTT. Independent A/B wall timestamps are not subtracted.
* A TCP RTT estimates are 10.609 / 10.545 ms, with zero retransmission-byte
  and timeout-episode increments. There are 61 successful socket-queue probes,
  no queued-byte positives, last empty observation at 610.245 ms and maximum
  sample gap 10.611 ms. Empty FIONREAD does not exclude bytes already held by
  TLS or an overlapped receive buffer.

The matching B preparation path is the dominant measured contributor to this
tail. It is not evidence of a 600 ms A receive-resume stall or a 600 ms network
round trip. B used a prepared candidate aged 1.402 s, but this does not mean its
authenticated native input/release pair was fully ready at entry.

B's retained startup decomposition includes service child-connect 200.329 ms,
bootstrap readiness 309.835 ms, worker installed validation 96.533 ms, agent
authentication 66.106 ms and desktop/pair readiness 127.295 ms. Native input /
release installed validation is 256.765 / 292.068 ms. These overlap and can
include earlier prewarming; they are not independent sequential contributions
to the current 582.901 ms wait. Further B preparation investigation should
establish which work was still unfinished when the entry arrived before
changing image/authority validation or worker lifetime policies.

## A execution trace

The slow-handoff snapshot was saved at 20:03:53.989 UTC on the g547 A run.
The retained trace spans approximately 20:03:45.551--20:03:53.891 UTC and
contains three completed grant handlers and ten complete wire reads. Fixed
control regions, return ordering and the local snapshot/profile times place
the final captured grant at the 612 ms exchange. This is local trace/profile
alignment, not a cross-host timestamp comparison.

For that grant, the wire reader's final Runnable-to-Running delay is
0.032477 ms; read completion to grant-handler start is 0.066277 ms. The grant
handler then takes 19.776468 ms, consistent with the profile's 19.508 ms local
capture activation. Across the retained wire-reader wakeups, the maximum
Runnable delay is 0.070186 ms; maximum enqueue-region time is 0.021745 ms.
Maximum retained stop-the-world GC pause is 0.092250 ms.

The surrounding wire read lasts 1516.948 ms, but includes intentional idle
before the source request. Its states are approximately 1210.643 ms network
wait, 306.205 ms waiting without a retained reason, 0.045 ms running, 0.022 ms
syscall and 0.032 ms runnable. Do not call that entire read a grant latency,
packet transit time or cursor freeze. Trace network wait includes peer work
and OS completion delivery; it is not a hardware packet-arrival timestamp.

The trace supports prompt A execution resumption for this captured tail.
The old g546 325 ms sample has no corresponding execution trace and cannot
be retroactively assigned the same B preparation cause.

## Normal operation restored

After preserving the trace, A was deliberately relaunched without the trace
environment flag. Its saved disconnect intent initially kept it LOCAL; the
local CLI connect command resumed it and B reconnected. Doctor confirms
healthy/CONNECTED, matching CLI/agent/UI/installed revision and running service.
Collection `1149e6ced8eabcadd5b20589bf654f90` confirms READY on both devices,
the same new connection, and no trace-enabled event on A's current run.
This intentional restart is separate from the physical-trial fault audit.

Verified exact build/connection identity, eleven matching complete exchanges,
measured-field masks, input hashes, trace decoding/state intervals, and final
CLI/input health. No source, API, DB, dependency, runner, or installation change
was made in this review. Source tests and CI were not rerun because this is
an evidence-only documentation change; native Linux was not part of this trial.
The remaining primary target is B's 582.901 ms readiness response wait.
