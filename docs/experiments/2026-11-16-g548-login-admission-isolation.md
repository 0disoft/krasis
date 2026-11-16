# g548 login admission and diagnostic persistence

The g547 matched outlier had B ReadyWait 582.901ms, login admission
219.744ms and request-read-to-grant-written 599.315ms. A's corresponding
written-to-grant-read interval was 611.914ms; the 12.599ms residual is not pure
network RTT. The final A reader runnable delay was 0.032477ms. See the
g547 paired-crossing report for the exact exchange and observation limits.

The login service's fresh console callback synchronously called
loginRuntimeRecorder.record. That method owns its mutex throughout protected
file validation, write and atomic replacement. Even an unchanged console state
could wait behind another record's persistence before deduplication. The
policy's grant read already runs outside the login gate, but its recorder can
still hold this separate mutex. This is a reproducible unnecessary dependency;
the historical profile did not separate mutex wait, WTS RPC and persistence,
so it does not establish which one consumed the original 219.744ms.

The callback now publishes one immutable latest console observation. The
existing policy record and final service record save it; no additional writer,
queue, periodic timer or retry loop is introduced. Success does not erase an
unpublished console failure. A failed save does not mark the snapshot persisted,
and a previously persisted console failure cannot overwrite a newer worker
failure. Fresh WTS observation, login-owner retirement and the required second
console observation after actual retirement remain synchronous and unchanged.

A virtual-time regression holds persistence for 220ms and verifies admission
completes with zero simulated elapsed time while the write remains outstanding.
It also rejects a failed fresh console query and checks the final persisted
failure. Other tests cover save retry and failure ordering. This is an isolated
dependency test, not a measured reduction of the physical 582.901ms tail.

Service-readiness indices 5/6 now separate login-gate lock wait from fresh
console observation, within the existing total admission stage at index 1.
The deterministic contention test uses an explicit test clock: 180ms gate
wait and 40ms observation remain distinct. An initial synctest-clock version
timed out because a mutex waiter is not durably blocked for virtual-time
advancement; the corrected test uses clock/barrier injection, not a larger
timeout. KRDA adds 16 bytes; legacy KRD1--KRD9 receipt lengths remain fixed.
Tests cover roundtrip, all truncation positions, reserved bits and the following
protocol frame. The existing readiness fuzz seeds include KRDA.

Native validation still hashes every image byte through protected pins. Native
startup spans can overlap prewarming and must not be summed. No hash backend,
digest cache, input authority, worker retirement or preparation lifetime change
is included. Remaining OS query and native readiness delays require fresh
g548 physical crossings; no unit test establishes their end-to-end improvement.

The runtime source of truth is updated in docs/architecture/02-runtime-flow.md.
Local readiness diagnostics are extended; no public CLI/API command, peer wire
protocol, DB, dependency, repository hygiene or CI runner configuration changes.
Rollback is a paired reinstall of the retained g547 setup and CLI reconnection.

## Validation and paired rollout

Source revision: `0d5553df9a661fb0784b697082177a6da74af3af`, generation 548.
All inputworker/loginagent/handoff/localapi package tests passed after stopping
the live agent, including the portable recovery ownership fixture. Its first
live-session run was rejected by the real ownership fence; no fence was bypassed.
Targeted inputworker race tests, vet (`-unsafeptr=false`, matching the native
interop check), diff whitespace checks and all 111 frontend tests passed.
Native UI build, embedded input installer, eight-file/icon check, archive
roundtrip, offline update verification and setup self-verification passed.
Remote CI and Linux VM testing were not run for this Windows admission change.

Before rollout, collection `6c1382d7fa5f606305e576bed8ec986f` found A's old g547
service stopped with error 1066; its protected runtime record reports a capture
worker missing final report (`worker-report-timeout`). That failure predates
g548 and is not claimed fixed by this diagnostic-persistence change. The record
was preserved. The exact new input installer's attended repair exited 0.
A's app was then installed first because its old management agent was unavailable.
The GUI setup's stdout/exit receipt was not retained by the launching shell;
post-install checks verified the intended app, UI, launch record, protected
agent revision and running service instead. Both protected-install diagnostics
report result 0, stage complete.

B was sent the update once: receipt `23a7d16cbb48204a4cd2d11499067e81` reached
UPDATED with exactly 29,238,025 / 29,238,025 bytes. CLI connect receipt
`1ece2268cf90ddfec6f5dcdf60609cbf` reports connected; B app-open receipt
`7fc62098d85a9e304fe275ffbb15d2e8` reports window_ready. A doctor is healthy,
CONNECTED and CONFIGURED with matching CLI/agent/UI/launch/installed revision.
Both devices advertise g548 at the intended revision. UPDATED is a completed
sender receipt, not an in-progress transfer; it was retained rather than reset.

Fresh collection `2fc97df951ecbce6b4078d254216a9f0` confirms READY and the same
connection on both devices. No post-connect timeout/fault/poison/reconnect or
recovery-required event appears in those short retained g548 runs. Physical
idle/fast crossing results and long-term recurrence remain unverified; the user
has been asked for the next trial. No execution tracing is enabled by this fix.

The subsequent user trial is recorded in
[the paired crossing review](2026-11-16-g548-paired-crossing-review.md): B
readiness max 28.489ms across twelve matched exchanges, no post-connect fault
in approximately 170 seconds, but stutter remains with A capture/resume and an
unresolved grant-wait residual. That review supersedes the pending-trial status
above; it does not turn the original persistence hypothesis into proven timing
attribution for the historical 582.901ms sample.
