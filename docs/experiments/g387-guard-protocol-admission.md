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

## Paired application

Both PCs applied 3996941d698009c1c55d992cc1f21f6e574894ee. UI's 85 tests,
native build, installer provenance and eight-file archive validation passed.
Update verify accepted 27,985,727 bytes; B receipt
b32f8ef8ec10697d5b5d4a3ddd3b44dc reports UPDATED. A installer returned 0 and
its installed agent SHA-256 matches. No Disconnect was sent.

Doctor reports healthy/CONNECTED with matching UI/agent/installed revisions
and completed stage cleanup. Collection c390bc2a157df6a7a31eadc81b21117d at
21:12:56Z/57Z reports READY input health and one protected source ready event
on each PC. Its 22 A / 45 B current-build events contain no fault_ or
session_failed. This verifies startup and short-window runtime, not a new
physical-input, lock/suspend or long-soak acceptance trial.

## Continuation

Two implementation tasks were completed in this round; the guard admission
task began before the ten-minute cutoff. No later task was started. All ten
issues remain open because their complete acceptance is not yet established.
The active goal remains resolving all issues, with the local-only/no-push
instruction intact. There is no pending local source edit and only main exists.

Next #50 work is the portable product path in product_runner.go that constructs
the in-process wininput adapter when installedProductInput returns nil. Runtime
joins that adapter synchronously, so a non-cooperative native injection still
owns runtime state. Preserve ordinary portable operation, input ordering,
pressed-state certainty and independent crashguard recovery while introducing
a genuinely owned cancellation/retirement boundary. Also audit synchronous
close/kill inside crashguard abortProcess; admission bounding alone does not
prove those native calls interruptible. Do not label either boundary fixed from
cooperative mocks or the protected-backend startup observations above.
