# g387: bound crashguard protocol admission after timeout (#50)

Process.invoke started a goroutine before its Client acquired the serial
protocol mutex. An operation that ignored pipe closure could keep that mutex
forever while each later caller allocated another blocked goroutine. A late
successful reply could also be accepted after the owner's abort was requested.

Process now admits one protocol operation through a cancellation-aware gate.
Its existing two-second exchange budget includes gate admission. Expired
waiters start no operation and do not kill the unrelated current owner. An
actual exchange timeout/abort permanently fences this Process. Subsequent
calls start no goroutine, and a late successful result cannot revive the
owner. Stdio/process-abort attempts and process Done remain separately owned;
timeout does not fabricate process exit or held-input recovery. Existing
stage-retirement gates still control replacement of unconfirmed children.

This is read/write IPC ownership, not detached native injection. The previously
started guard call is retained until it returns. A native close/kill operation
that itself ignores cancellation is not newly proven bounded by this change.
Portable in-process SendInput remains separate #50 work.

## Validation

Crashguard and residentagent suites and crashguard vet passed. Controlled-time
tests hold one operation beyond timeout despite pipe closure and failed process
termination, submit twenty concurrent waiters and twenty later calls, and prove
that only the original operation runs and termination is attempted once. Late
success never reopens admission. Another test proves admission timeout starts
no IPC, cannot abort the current owner and allows later fresh admission.
The real hung-helper subprocess test and existing bounded-close/ExitPending
tests remain in the executed crashguard suite.

No race detector (CGO/compiler unavailable), hosted CI or new physical fault
matrix was run. Paired application is recorded after verification. No public
API, DB, runner, dependency, firewall, signing or trust-policy change. Local main
only; no push, extra branch/worktree or GitHub comment.
