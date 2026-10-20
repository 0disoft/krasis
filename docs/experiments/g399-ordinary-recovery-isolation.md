# g399: isolate ordinary installed release recovery (#50)

The ordinary installed journal no longer performs synchronous native authority,
desktop or release calls while it owns pressed state. Before readiness it starts
two independently authenticated fixed native children: live input and release
only. The journal keeps network framing/order and authoritative pressed state.
Input must fully retire before any recovery release; the release child must also
fully retire before the final service receipt. Missing exit/IPC proof remains
ErrRecovery even with an empty journal, and uncertain retirement is not retried.

The release-only decoder accepts heartbeat and key/button-up only. It rejects
down, move, wheel, cursor and arbitrary operations before native execution. Its
release bypasses a child-local pressed ledger because the parent owns that
ledger. It cannot acquire a separate native guard journal; guard-side native
release is denied. Failed/partial/unknown native outcomes remain unchanged.

The fixed --native-release-worker mode validates the protected installation,
SYSTEM/UIAccess console token, actual service and agent pipes/images, exact live
parent and immutable lease bootstrap. Every release checks the original console
logon and active desktop. Existing key-up authority survives agent exit and lease
expiry. An exact service process handle is pinned during full startup SCM/pipe
authentication; cleanup permits that same service's StopPending lifetime while
requiring its exact live process and pipe. No new press authority is granted.

The ordinary job permits precisely three processes (journal/input/release), all
under kill-on-close. Login jobs remain at two and retain their prior recovery
path. This adds one prestarted process to an ordinary active lease and extra
startup authentication. Boundary latency is not inferred from unit timings.

The ordinary parent needs no native desktop binding; native startup/readiness
comes from both authenticated child transports. Recovery IPC and process cleanup
are bounded. On an unacknowledged release the parent preserves Remaining, closes
the release child and reports recovery failure, without replay or local fallback.

## Verification

Inputworker, residentagent, application, peer, input command and agent command
tests/build checks and relevant vet pass. Real hidden subprocess fixtures cover
successful release and a native release that ignores cancellation. The actual
ordinary journal entry is used with parent authority queries that always fail;
it performs no such query, retains held input when release is unacknowledged and
waits for exact input/release process exit. Empty-journal uncertain release-child
retirement stays fenced. Windows tests verify the three-process ordinary bound,
unchanged two-process login bound, protected-mode rejection and exact service
lifetime checks during simulated StopPending. These fixtures inject no native
keys/buttons and do not prove physical held-input or Winlogon acceptance.

The release-only transport was a separate preceding local commit ba79d9a. This
is the second implementation unit, started before the ten-minute cutoff. #50
remains open for login recovery and final call-graph/device acceptance. #59,
#70 and hardware/UI/distribution gates retain their own requirements. No public
API, DB, dependency, runner, firewall, signing, issue comment, branch/worktree or
push changes. Paired installation and ordinary smoke are recorded when verified.
