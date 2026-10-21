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

## Paired application

After correcting the advisory/ACK query-lane regression found by the full suite,
inputworker/application/peer/residentagent/productpanel suites all pass, as do
inputworker vet, UI 85 tests, native builds, embedded installer provenance and
eight-file bundle checks. Offline verification accepted g407 revision
ba1a7f1ef5873a482b6794d853a163e987f5c529, 28,299,127 bytes. The independent
query-retirement unit is b1dfa7a; writer/admission integration is ba1a7f1.

A installed at 01:19:20Z with complete/result 0, no recovery needed, and its
installed-agent SHA-256 matches the bundle. B update
6d798ae5f584e3ff5504448443481aad reports UPDATED; B's installation at 01:19:39Z
also reports complete/result 0 with no recovery needed. No manual reconnect or
installer retry was required.

Collection b9fc013bd79d15b01028c473d39be9ff overlapped reconnection. Fresh
collection 10dd045d4bb5649cd3a0f991c7ed512e at 01:20:48Z/50Z contains 36 A / 37 B
current-build events, CONNECTED on both, READY service/input health, and no
current-build fault or startup-failed code. A's fresh doctor independently
reports healthy CONNECTED, matched CLI/UI/resident/installed revisions, verified
launch record and completed update cleanup.

Standalone installed-input smoke was not repeated alongside the running normal
resident, whose singleton conflict was already observed in g405. No physical
held-key, security desktop, power, DHCP, home-pair, DPI/accessibility, soak or
signed-release acceptance is inferred. Race detection was not run with CGO
disabled; hosted CI was not run because commits remain local.

Both implementation units started before this continuation's ten-minute cutoff;
no new unit was started afterward. Final remote inspection still has open issues
#4/#5/#6/#7/#15/#16/#20/#50/#59/#70. It also found dependency PR #91 (npm group,
two updates), which is queued for the next continuation rather than starting a
new unit past the cutoff. No dependency dashboard was changed. Read-side
authorize/present proof ownership and native hook callbacks remain next under
#50/#59. Earlier physical-pair/signing questions still need external evidence.
