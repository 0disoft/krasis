# g417: capture local handoff stalls with a fast network exchange

## Actual g416 trial

The installed g416 A UI and agent were gracefully restarted using the verified
launch record and pinned runtime exit. No executable was replaced. A's agent
logged `runtime_trace_enabled` at 09:06:27.0123177Z on 2026-10-21, then connected
to B. The first restart helper stopped after the already-idle agent returned
`no_active_session`; accepting that terminal idle result allowed the same
verified restart to finish. No forced termination was used.

The user reported slight stutter without a major problem. Paired collection
`8885f0e5d856e81e6fdaec9c9404d56c` at 09:07:50Z contains 12 completed source
profiles on A (807 counted frames) and 12 receiver profiles on B (778 counted
frames), all ending `ok`. Receiver profiles identify the native backend; this
does not prove the installed protected backend's fault acceptance. Profile
counts reflect local/coalesced observations, not a packet-loss comparison.
There is no fault/failure in either exported event history after 09:06:49Z.

A's one-minute summary measures maximum queue residence 70.742ms and control
dispatch 225.175ms, with timer lateness 34.632ms. B measures maximum input
injection/dispatch about 9.056ms and timer lateness 6.871ms. These software
timings do not measure physical input-to-display latency or establish why the
user perceived stutter.

The third source profile has request-to-grant exchange 35.488ms but local
capture activation 223.420ms. Another profile has source deactivation 137.952ms.
Neither the >=80ms exchange trigger nor >=1s timer trigger requests a snapshot
for those cases. A's only trace remained the older October 19 snapshot; the live
g416 recorder also produced no save/expiry event during a bounded observation.

## Change and verification

After summary emission, request the existing opt-in snapshot when a measured
source capture activation or deactivation span reaches 80ms. Preserve the
network trigger. Each span qualifies independently; do not add nested spans,
infer measurements from nonzero values, or use intentional inter-input idle.
The input/cleanup paths acquire no new lock, perform no trace/disk I/O, and
receive no new diagnostic callback. The existing recorder remains opt-in,
single-use, 15-minute bounded and local-only; no raw trace enters peer export.

The regression uses the observed 35.488ms network / 223.420ms capture case and
the 137.952ms deactivation case. It also checks missing measurement bits,
receiver exclusion, the existing network threshold, and no sum of separately
subthreshold capture spans. Residentagent, handoff and runtimestall suites pass.
This fixes a diagnostic blind spot; it does not claim the stutter or trust
freshness/held-input failure is fixed. No issue is closed from this change.
No public API/DB, wire format, dependency range, runner, firewall, signing or
remote-ref change. Paired bundle/application evidence follows below.
