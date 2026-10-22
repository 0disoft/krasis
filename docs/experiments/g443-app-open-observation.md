# g443: bound app-open observation independently of native replies

PeerAppButton checked its 70-second deadline only after awaiting status. A
native call that never returned therefore held the UI busy beyond the stated
observation window. Its timers, component state and bridge I/O also lived in
one Svelte script, preventing direct deterministic lifetime tests.

Extract a controller with optional monotonic clock, scheduler, AbortSignal and
view callback. A separate deadline bounds both initial launch observation and
later status polling. Expiry retains outcome_unknown or window_unconfirmed;
it does not claim cancellation of the native action. Dispose removes timers,
the signal listener and view callback; late replies/rejections cannot revive
the old view. Native calls keep the existing bridge's per-peer execution fence.
An expired persisted receipt permits bounded native confirmation, never an
automatic replacement launch. Existing bridge signatures and result schemas
remain unchanged. The Svelte effect owns controller lifetime for the peer.

Regression coverage uses a fake clock/scheduler and the real bridge: 69,999,
70,000 and 70,001ms, hung initial/status calls, transient queries, shorter saved
budgets, locally expired receipts, abort/dispose, synchronous failure, late
success/rejection and no replay after observation expiry. UI timeout cannot
release an outstanding native execution lock. No input behavior, API, DB,
dependency, runner or repository hygiene policy change. Paired rollout and
validation results follow separately. Local main only; no push.

Passed: 100 frontend tests (nine new controller tests), native frontend build,
agent 1.976s, loginagent 2.433s, residentagent 6.317s, package smoke 8.43s,
eight-file inventory, offline verification and diff check. Archive 28,363,539
bytes at 7c047c694ccec9b6b9bd4f3839c304d84cbc1060. No race detector, full
unrelated Go suite, live stalled app-open invocation or long soak; the fake
clock tests exercise the actual bridge without inducing a stuck native process.

B update b9555f731376d4725921004133862ba6 reached UPDATED. A acknowledged
safe-stop before the fixed installer repair exited 0, then app repair returned
recovery_ready and the verified replacement resumed B. Collection
4e011131d7d64d49987db530cc7b85c0 at 16:03:00Z/16:03:03Z confirms both
installation receipts succeeded and both input health/services READY. Final
doctor is healthy/CONNECTED; peer status is ONLINE at matching g443.

Before deployment, g442 collection 3772c3078da242848652ff6390952896 already
contained three boundary-gate failures at 15:58:01Z/15:58:10Z/15:58:16Z and a
heartbeat reply-write failure at 16:00:18Z. Native/sender owners joined and the
service remained READY. These changes do not establish a fix for that separate
contention. Keep it as next priority, followed by queued UI cancellation and
the update-wait completion/cancellation boundary. Paired results:
results/2026-10-23-g443-testable-observation-and-jitter.json.
