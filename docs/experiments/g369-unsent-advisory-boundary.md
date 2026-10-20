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

## Installed acceptance

Revision `41fe24aa184bb7a47e4eefe1c00df41a21aa5204` passed all 85 UI tests,
native builds, installer provenance and the eight-file update package checks.
Offline verification accepted the 27,784,933-byte g369 bundle. A's installed
agent hash matches the bundle; its running UI, agent and recovery record all
match g369. B transfer `edff563874248fad5b92c4394de51615` reports UPDATED
at that same revision.

Final collection `0c18e80d8b1d590180bd028296f5008c` at 18:04:03Z/02Z
(2026-10-21 Korea time) reports input health and service READY on both PCs,
with no new fault, session failure or source-command/heartbeat failure for g369.
A doctor is healthy/CONNECTED and B is ONLINE. The observation covers about
two and a half minutes after connection, not long-term stability or the complete
hardware matrix. Remaining #50/#59 native-stall/hook-liveness work stays open.
No issue comments were added during this execution and no Git push occurred.
