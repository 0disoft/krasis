# g407: bound source writer admission and native checks (#50/#59)

SourceWriter.SendAction checked cancellation only at entry, then performed direct
native full/presence checks and uncancellable writer waits. A controlled 50ms
caller regression returned only after the native fixture's one-second release.
Boundary sender shutdown also joined this same unbounded authorization path.

The writer now applies its existing one-second budget to the whole operation:
fresh full authorization, immediate gate admission, fresh presence and bytes.
An earlier caller/worker deadline or worker cancellation wins. Queued waiters use
the existing cancellation-aware mutex. Native full/presence work uses the bounded
Channel query owner, preserving fixed source diagnostic operations/numeric errors
and the existing four-task process limit until actual resource retirement.

Advisory boundary full checks use the existing maintenance lane independently
of input/command full checks, so a blocked advisory query cannot hold a command
ACK behind it. Both still require their own fresh full proof. Boundary batches
now discard full authorization after gate contention, matching ordinary source
writes; a previous proof cannot survive an intervening writer. Every original
bounded packet keeps its binding/order and consecutive sequence.

Joined retirement and already completed command results may report after capture
cancellation under their own one-second budget. Neither can authorize new input.
An expired lock waiter submits nothing and consumes no sequence. Actual attempted
write failures still poison the stream; only existing native pre-submission proof
permits advisory drops or later terminal notification without sequence gaps.
No input/native command is run in a detached query worker.

## Verification and limits

Focused writer/query-retirement tests and vet pass: 50ms full/presence expiry,
gate admission and combined preflight/write budget, retained native resources,
worker cancellation despite a live caller, cancellation-independent joined
retirement, fresh boundary authorization after contention, command/heartbeat ACK
independence, sanitization of native failure details, and existing native
pre-submission/attempted-write/retirement sequence cases. Fixtures that notified
only once were made safe for legitimate reauthorization after contention. The
command-ACK test permits additional fresh checks but still requires independent
authorization, exact packet order and timely ACK delivery.

The independent query-retirement fix is documented in
authority-query-retirement-ownership.md and included in this bundle. Read-side
proof/authorize callbacks and hook native desktop/cursor calls remain for #50/#59;
their shutdown joins and physical held-input acceptance are not inferred from
writer/query adapter tests. No public API/DB, dependency, runner, firewall or
signing-policy change. No push, issue comment, branch/worktree or remote ref edit.
