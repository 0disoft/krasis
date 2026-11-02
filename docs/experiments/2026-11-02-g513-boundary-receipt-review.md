# g513 paired crossings and boundary receipt failure

Authenticated paired collections `e607ff17cdd7309de652cc56d04bebca` and
`c6c3cac9c3313a48eefe8d54a4461f85` cover build
`6d02cb936afdb0420a82413ba13746f267a24760`. The later collection includes the
A failure that was not yet in the first local snapshot. Both protected input
services were READY. Historical `lastFailure` is not a new recovery incident.
Exact-build events are deduplicated by run/sequence/code. Only complete successful
source/receiver pairs with a matching exchange tag enter latency statistics.
No clocks are subtracted across machines; these are software spans, not physical
mouse-to-screen measurements or a controlled g511/g513 benchmark.

Nine complete A-to-B pairs:

| Local duration | Median ms | Maximum ms |
|---|---:|---:|
| Source request to grant | 44.302 | 66.063 |
| A capture activation | 13.563 | 35.684 |
| B preparation | 27.416 | 34.853 |
| B first input | 2.018 | 5.196 |
| B worst input per entry | 7.691 | 14.294 |

Nine **first-input** samples retained the new native IPC split:

| Native phase | Median ms | Maximum ms |
|---|---:|---:|
| Parent admission/submission preparation | 0.027 | 0.035 |
| Request submission to child read | 0.216 | 0.491 |
| Child admission | 0.002 | 0.025 |
| Child handler through reply encoding | 1.258 | 2.180 |
| Reply ready to parent's first read return | 0.057 | 0.770 |
| First read return to matching decode | 0.026 | 0.093 |

The retained worst-input samples were not detailed probes. Consequently these
first-input phases do not locate the 14.294ms worst input or disprove rare large
B stalls. The earlier 1,116ms B response stall did not recur in these nine pairs.

## One observed failure and one correction

At A-local 10:10:46 UTC, `installed_source_command_preflight_expired` preceded
a triggering `source-hook` timeout and `fault_source_hook_failed`. The source
had already been running for about 328 seconds; its main loop's only ordinary
command is the boundary receipt, not startup. This associates the failure with
receipt preflight. B recorded `remote EOF` on the same hashed connection and
subsequently reconnected. This is one connection failure, not proven service
error 1066. A's later CONNECTED record is at 10:11:00 UTC.

A's pre-trigger local GC trail contains a 1,488,894us interval with 1,392,189us
sampling lateness, a >=1s GC-pause observation, and a >=1s GC-stopping observation.
This establishes a local runtime stall near the 1s unsent receipt deadline. It
does not identify the Windows thread/runtime mechanism behind that stall.

g514 confines the correction to boundary credit receipts:

- One ordered receipt worker replaces inline waiting in the heartbeat loop.
  The existing one-entry queue, exact reference, wire order and reply matching
  remain. Its cancellation/join precedes Stop and native resource retirement.
- One receipt has an absolute 2s total budget, using existing IdleTimeout. Each
  attempt keeps the original command timeout. A 10ms backoff applies only to a
  producer-issued marker proving that no sequence/bytes escaped. Attempted,
  partial, unknown, protocol and authority failures are never retried.
- An unsent presence query owns an independent optional receipt lane and still
  holds its native handles until return. It uses the restricted three-query
  pool, leaving the fourth slot available to heartbeat presence. The process
  maximum remains four; Close includes the receipt lane in retirement checks.
  Query completion is published only after returning its capacity reservation;
  native work/retirement still finishes before either operation.
- Receipt proof obtains one fresh native presence check before immediate wire
  admission, followed by the same immutable-owner fence used by heartbeat.
  Client-side native queries per successful receipt fall from two to one.
  Receipt traffic grants no capture or input authority. Input/activation full
  SCM/WTS checks, native protocol, lease expiry and real wire failure policy are
  unchanged.

Deterministic Windows tests reproduce a 1.4s unsent presence stall. An independent
heartbeat is acknowledged at 100ms, then the exact receipt is transmitted once
within 2s. Separate tests verify fresh proof after the deferred native return,
no retry of ordinary deadlines/attempted writes, unchanged absolute retry budget,
cancellation-before-send, completion of already started requests, and deferred
native-handle closure exactly once. Physical recurrence after g514 still needs
a new crossing observation. No permanent reliability claim is made.

Validation: full race-enabled tests passed for inputworker, agent, residentagent
and productlog; go vet and diff checks passed. The broader race run also exposed
test-only races in the existing reentry fixture: shared build/first-candidate
observations now use atomics, and its worker is initialized before its expiry
timer is armed. No reentry production behavior was changed.

API/DB/runner changes: none. Source of truth: this experiment and the local
source receipt implementation; no product authorization or remote API changed.
