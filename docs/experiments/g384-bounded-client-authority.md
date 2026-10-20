# g384: bound client-side native authority queries (#50)

Input-worker clients previously performed synchronous SCM authorization before
pipe submission. The existing two-second context rejected a late result but
could not return to the receiver while native code ignored cancellation.

Each channel now owns at most one read-only authorization query. Every command
obtains a fresh result; a contended serial input gate still discards the earlier
result and reauthorizes. Client construction has the same bounded query budget
and rechecks the lease after authorization. No injection is detached or replayed.

Expiry permanently fences that channel's proof and returns not-applied without
consuming a sequence or submitting a request. Late native success cannot admit
input, heartbeat or capture. Close reports unconfirmed cleanup while a query is
still active, retains both pipe and proof handles, and sets wire deadlines.
The query owner closes those resources exactly once after native code returns.
Actual worker recovery and any uncertain pressed journal remain independent.
This does not reinstate the rejected g381 console-worker retirement proposal.

A process admits at most four native authorization queries, including expired
queries awaiting native return. Replacement channels fail closed when capacity
is exhausted. Permanently stuck calls retain a bounded set of resources until
process exit rather than losing ownership or admitting unlimited replacements.

Query diagnostics use a separate sample. Only a joined, unexpired result merges
its fixed timing/cycle fields into the calling input sample. Late native writes
cannot race with completion of that sample or alter an already emitted report.

## Local validation

Passed inputworker, handoff and residentagent suites and go vet for those
packages. Focused controlled-time cases prove deadline return while native code
remains blocked, not-applied certainty, unchanged sequence, no replay or late
proof reuse, bootstrap bounds, heartbeat progress before expiry, fresh proof
after input-gate contention, exact-once cleanup, capacity exhaustion across
replacement channels and capacity recovery only after native completion.

A Windows test retains a real named pipe and limited-query process handle
through caller cancellation. A late kernel PID/process query succeeds before
the owner closes the handles; the expired proof remains unusable afterward.
No input is injected by that test. Diagnostic sample isolation has its own test.

Race-detector execution was attempted but unavailable because CGO is disabled
and no gcc/clang is available on PATH. No compiler was installed for this task.
Hosted CI and the physical lifecycle/fault matrix were not run. Paired package
application and current-build diagnostics are recorded separately after rollout.

## Remaining issues

#50 remains open: native injection/console-worker retirement that ignores
cancellation still needs a separately verified solution. #59 mandatory callback
desktop/cursor work, #70 physical endpoint changes, and #4/#5/#6/#7/#15/#16/#20
hardware, accessibility and signed-release gates remain. The g377 user report
confirms ordinary paired input only and does not substitute for these gates.

No public API, DB, dependency, CI runner, firewall, trust-policy or signing change.
Local main only; no push, issue comment, extra branch or worktree.
