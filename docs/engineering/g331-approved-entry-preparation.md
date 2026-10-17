# Approved entry finishes bounded in-flight preparation

Collection `eaf6e89f0c4f0444ddaf0613da77c76e` contains matched g330 receiver
authorizations near 20/25ms with prepared children and 180/183ms with
PreparationNoCandidate. The B control dispatch sample is 184456us. Missed
preparation records precede those two cold lease-ready records. The receipt does
not preserve the cancellation reason; not every missing candidate is proven to
come from this race. Freshness, missing hints and completed expiry remain other
possible causes.

The local reproduction makes the broken ownership sequence deterministic:
authenticated optional construction has started, an independently approved entry
arrives, `open` cancels it before the old 300ms preparation deadline, cleanup
destroys the candidate, and `connect` must construct another child. The original
code fails the test immediately with a discarded in-flight candidate. With the
patch, the same construction finishes after 120ms on a virtual clock and the
ordinary lease connection claims it once.

## Bounded joining, unchanged authority

The preparation owner now publishes its immutable deadline and completion
channel with its existing cancellation function. An approved entry may join
only a currently running preparation with a positive remaining deadline no
greater than `agent.InputPreparationTimeout`, the receiver's existing 300ms
budget. It does not create work or extend either deadline. Missing/expired/long
deadlines keep immediate cancellation. Entry cancellation cancels preparation
and cannot start a service connection. Shutdown still cancels rather than joins
optional construction. No new background goroutine or timer is added by joining.

Pending-entry priority and the mutex remain: new hints cannot enter, native
cleanup is joined before a lease or pin disposal, failed preparation uses the
normal connection path, and service expiry/session/one-claim checks and exact
image/agent/lease authentication are repeated. A readiness failure may consume
its remaining original budget before cold fallback, up to 300ms; this tradeoff
needs paired-device latency validation. Work whose native cleanup does not
return cannot be bypassed, as before.

The explicit join is inside the measured admission lock-acquisition interval,
while preparation owns the same mutex. Existing OpenLock/preparation-overlap
records therefore preserve this wait. The allowlisted
`installed_input_preparation_joined` event means an entry waited for that
attempt's completion; StartupProfile.Prepared/PreparationDecision separately
determine whether the service actually reused its candidate. No raw error,
pointer position, key, PIN or process metadata is added.

## Verification and rollout

Virtual-clock tests reproduce discard-before-reuse, validate caller cancellation
with native cleanup still fenced, reject waiting on unapproved leases, and verify
the unchanged original timeout with normal fallback. Existing unbounded-owner
priority and shutdown cleanup tests remain. No prepared-child lifetime, launch
cooldown, GC policy, input deadline, lease lifetime or input replay rule changes.
No public API, peer/service wire, DB, dependency, ACL or runner changes. The
internal preparation timeout is shared by receiver and admission to prevent drift.

After updating both PCs, compare matched cold/resumed entries and rapid reentry,
then lock/PIN transitions. This local test establishes the ownership fix, not a
measured device speedup. Rollback is an ordinary peer app update; no persistent
state migration is needed.
