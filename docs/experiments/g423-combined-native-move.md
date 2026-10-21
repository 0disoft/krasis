# g423: combine native movement and cursor readback

The user's severe B-side stutter report follows g422. Paired collection
`18c1fc79f0a2c226a55853a31c4f059f`, exchange
`7c16ae06cd06b17291dfb9250c49ae6add35eab8d9ba61663c868b1764b5698f`,
contains a B receiver sample with client total 280150us, worker apply
135626us and cursor observation 114668us. These nested software spans are
not an end-to-end physical latency measurement.

The journal worker decomposed opMoveRead into native-child opMove followed
by native-child opCursorRead, despite the child already supporting opMoveRead.
Use that existing combined command only for the concrete isolatedInput owner.
Each move now needs one child exchange and one child desktop admission rather
than two. Input still executes before real cursor readback. The child retains
its fresh authority/desktop checks, sequence/binding validation and failure
certainty; no input retry or cached cursor observation is introduced. Pressed
input journaling, child retirement and independent release recovery are intact.
The parent profile records the combined exchange as one containing apply span,
without inventing separate native-stage measurements.

A regression exercises actual child request/response encoding, verifies one
exchange, desktop admission, move and readback, and preserves unknown outcome
after readback failure without replay. Existing direct-native readback/retry,
submission and isolated-worker recovery regressions also pass. Full inputworker
suite passes in 24.879s. Workspace packaging passes 85 UI tests, native frontend
and Windows builds, embedded installer build, update archive smoke (6.43s)
and eight-file bundle verification. Offline verification accepts generation 423,
revision `e1b17ef59b2321447d1f8dc5555d9323a50173fe`, 28162923 bytes.

A fresh pre-change doctor reports UI RUNNING, session FAULTED and protected
service STOPPED. Source response-reader backpressure and active-heartbeat
retirement remain separate investigation candidates; this change does not
claim to fix the service's recurring 1066 or reconnect failure. Do not close
those issues based on this optimization. No public API/wire schema, DB,
dependency, runner, firewall or signing changes; no remote push.

## Paired application

B update `07db2ddd9e3ff85798437b31951fc5b3` reaches UPDATED at the exact
revision; its install receipt is 11:12:53.860207Z, result 0. A's existing
safe-stop completes and g423 repair receipt is 11:13:14.0481259Z, result 0.
The old g422 management UI prevents ordinary replacement and logs
ui_replacement_launch_failed. After verifying its exact executable path and
retaining its process handle, only that old UI is terminated; the new UI is
opened. The g423 installed agent is started and connect B completes.

At 11:16Z A's doctor reports healthy CONNECTED, matched g423 UI/agent/CLI and
installed agent, running protected service, and no next actions. Input health
is READY; B's collected service and input health are also READY. Collection
`06dba51a6320161ca2e508f610e1e63a` preserves the paired installation receipts.
No forced agent/input-worker termination is used. A's UI is left open.

The user has been asked for a short B-side movement comparison. Until that
report and matching runtime samples exist, the confirmed effect is eliminating
one child transaction per move, not a measured physical latency improvement.
Hosted CI, signing and the full device acceptance matrix were not run.
Remaining priority: deterministically reproduce source reader/backpressure
blocking heartbeat replies and verify bounded orderly retirement on failure.
No new implementation started after the ten-minute cutoff.

## User comparison: stutter remains

The user reports "similar" after the g423 B movement comparison. Treat the
stutter objective as unresolved. Fresh paired collection
`2474b2da442e3d9162199aa7980c898f` retains four matching exchanges.
For `a8371c0fc9beb1e5f34f5b6defd555feea73f5766f0b36f89e2ef8ea1044a340`,
A source worst dispatch is 1186284us, including write 915012us; B receiver
worst dispatch is 161486us. Those are independent worst operations within
the same exchange, not one matched event or additive end-to-end measurement.
For `3ca275652bc4e442b5a412012760de685f06c3b4dcbb8661918258cefe7dbfa2`,
B receiver worst dispatch is 528452us, client authority 2825us, response
525111us and containing worker apply 133940us. The combined worker profile
has only slot 0 measured, confirming the intended combined path ran.

A source TCP RTT samples change from approximately 11-17ms in the earlier
exchanges to 107-120ms in the later exchanges, without observed retransmitted
bytes or timeout episodes. This does not distinguish receiver backpressure,
network delay or scheduling. Do not infer a physical latency distribution
from these samples or subtract nested software spans to claim native latency.
Final doctor remains CONNECTED with matched g423 UI, agent and installed
agent, running service and READY input. No fresh session fault is recorded
in this comparison; no reconnect/soak acceptance is implied.

Next priority is B's long ACK response interval versus the journal worker's
combined native-child exchange, together with A's long write interval. Verify
read/dispatch/write ownership and native-child admission using a bounded
reproduction or trace before another rollout. The advisory review's proposed
source-reader starvation is a hypothesis: SourceSession.Offer reserves channel
capacity under its queue mutex, so it must not be described as an unbounded
queue send without a concrete reproducer. Preserve ordering and held-input
recovery, and do not replace missing evidence with widened timeouts.
