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
