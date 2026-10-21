# g406: bound source client command checks and gate admission (#50/#59)

The receiver Client was bounded in g405, but sourceClient.call still performed
direct full authority queries before taking its IPC mutex, a direct presence
query under that mutex, and uncancellable mutex waits for both commands and
shutdown. A 50ms activation regression returned only after the fake native query
was released at one second. The pre-fix mutex regression also stopped making
progress; its own test process was terminated, not a product process.

Source commands now share the Channel-owned bounded full/presence queries. The
existing command budget covers native preflight, lock admission, wire and ACK:
one second normally, the existing two-second activation budget, and the existing
RecoveryTimeout plus one second for stop. Earlier caller deadlines still win.
The serial source-client gate uses the existing cancellation-aware mutex. A
waiter that expires without starting native work leaves the owner intact. An
expired in-flight command query fences the channel, preserves its native handles
until actual return, submits no request and clears capture authority. No sequence
or epoch is created from late success; proof is discarded after contention.
The command timing sample now includes its final presence query as well.

Optional heartbeat preflight has an independent, single-query lane using the
same process-wide four-task bound. Its existing one-second limit now actually
bounds native presence work. Expiry alone sends nothing and does not retire a
healthy command owner, consume a sequence or renew worker liveness. A later poll
cannot multiply the stuck native task or reuse its result. New commands still
require fresh independent full and presence proofs. Explicit shutdown fences all
lanes and cannot confirm resource retirement until every native owner returns.
Heartbeat proof admission rejects fenced/closing authority without waiting on a
pending writer. Actual sent heartbeat failures and existing worker idle limits
retain their previous fail/recovery behavior.

## Verification

Full inputworker tests and vet passed. Focused virtual-time regressions verify
50ms command native/presence/gate expiry, no wire submission/sequence/epoch,
retained native handles, optional heartbeat expiry with one retained task over
repeated polls, and final resource closure only after all three native query
owners return. Existing heartbeat/activation independence, post-contention fresh
authorization, source idle/ACK bounds, proof binding/single-use/fencing and unsent
admission tests pass. The receiver regression still verifies two real worker
heartbeat writes during a stalled full query. Canceled native queries now retire
their owner after completion, so the older source tests were updated to assert
this ownership behavior rather than preserve a canceled proof.

No hook/input command was moved into an unbounded goroutine. These controlled
query/pipe tests do not prove physical held-input or secure-desktop retirement.
Remaining direct source-writer, worker-side proof and native callback calls are
outside this client unit; #50/#59 remain open for their call graph and physical
acceptance. No public API/DB, dependency, runner, firewall or signing policy
changes. No push, remote ref changes or issue comments.

## Paired application

Application/peer/residentagent/productpanel suites, UI 85 tests, native builds,
embedded-installer provenance and eight-file package checks pass. Offline bundle
verification accepted revision 0f6d17417f925993aa92429d7b4b00edc090b024,
28,302,519 bytes. A's verified UI replacement installed g406 at 01:02:32Z with
complete/result 0 and no recovery needed. Its installed agent SHA-256 matches
the bundle. B update 9a85a3369b3c9842d952d728042f5872 reports UPDATED; B's
installation at 01:02:58Z also reports complete/result 0, no recovery needed.

The first collection ce03cfd19096036878103b58b35c9b87 overlapped reconnection and
was not used as proof of completed connection. Fresh collection
e2ceb9e1f4becafee7422d10dc55c79d at 01:03:51Z contains 33 A / 32 B current-build
events, CONNECTED on both devices and READY service/input health, with no
current-build fault or startup-failed event. A's fresh doctor independently
reports CONNECTED, matching CLI/UI/resident/installed revisions, verified launch
record and completed stage cleanup. No manual connect or installer retry was
required for this application.

Standalone installed-input smoke was not repeated while the normal resident was
running; g405 had already established the singleton conflict. No held-key,
Winlogon, power, DHCP, home-pair, DPI/accessibility or signed-release acceptance
is inferred from these checks. CGO disabled: race detection not run. Hosted CI
not run: commits remain local as requested.

No further implementation unit was started after this continuation's ten-minute
cutoff. Next: #50 sourceWriter's direct full/presence queries and worker-side
authorize/present callbacks, including shutdown joins; #59's hook callback native
desktop/cursor dependencies remain separate. #4/#5/#6/#7/#15/#16/#20/#50/#59/#70
remain open because their evidence is incomplete. The earlier device-pair and
real signer questions remain unresolved. No branch/worktree changes or push.
