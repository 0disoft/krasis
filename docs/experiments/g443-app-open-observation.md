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
