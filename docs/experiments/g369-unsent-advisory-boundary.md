# g369: do not retire capture for an unsubmitted advisory batch

The g368 writer kept its sequence after a native pipe proved that a boundary
batch expired before WriteFile. However, sendBoundaries still returned that
error to pumpSourceBoundaries, which canceled the capture owner. A disposable
pointer observation could therefore stop the heartbeat reader and trigger
recovery without sending any bytes. This is reproduced at the native pipe
boundary; it is not proof that every observed g368 IPC failure used this path.

g369 discards only advisory boundary batches with the native unsubmitted-write
proof and a deadline-expired cause. This matches the existing bounded queue's
ability to drop advisory pointer observations. It does not grant a boundary
transition, renew a lease or claim an ACK. The next observation still requires
normal authentication and admission. The batch is never replayed.

Attempted zero-byte writes, partial writes, configuration errors and authority
failures keep the existing fatal result. Input actions, command replies and
retirement frames do not acquire this advisory-drop policy.

Native pipe regression verifies the expired batch consumes no sequence and the
same owner can send the next exact heartbeat ACK, followed by valid retirement.
Separate tests verify attempted zero-byte and partial batch failures poison the
stream and prevent another write. Full inputworker, residentagent, peer and
localapi tests pass. Package and installed acceptance are verified separately.

No public API, DB, dependency or CI runner changes. No Git push or public
release. Remaining hook-liveness, native-call cancellation and hardware gates
are not completed by this narrow fix.
