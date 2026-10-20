# g395: preserve source startup cleanup failure

A valid, authenticated source retirement with failed status was converted to
ErrProtocol during startup, whereas the live reader preserves ErrRecovery for
the same response. Startup now retains ErrRecovery, without granting readiness,
clean retirement or a retry. Wrong binding/sequence and malformed frames still
fail as protocol errors.

Startup diagnostics also retain a fixed outcome category alongside the existing
stage and Win32 category: cleanup, EOF, truncation, timeout, closed, cancellation,
protocol, authority, emergency or other. Existing live-reader classification is
shared unchanged. Neither native error strings nor input contents are logged.

Inputworker, productlog, agent and residentagent tests pass. Tests distinguish
authenticated cleanup failure from wrong-channel frames, reject false clean
retirement and cover fixed categories including private-error redaction. This
does not establish the underlying cause of B's g393 source startup failure or
complete #50/#59. All ten open issues retain their remaining acceptance gates.

The controlled g394 disconnect/reconnect at 23:18:54Z yielded clean joined
source retirement on both PCs. Explicit Disconnect intentionally stopped B's
automatic resume, so A alone then waited for B's explicit Connect. This is not
a startup failure reproduction. B reconnection was requested from the user;
do not describe CONNECTING as healthy or the test as a completed reconnect.

No push, issue comment, branch/worktree, dependency, public API, DB, runner,
firewall or signing change. The private source wire format is unchanged.

## Application

Both PCs applied generation 395, revision
51fd1c08ab09c986d433cbc7541e6d9a250018de. A installer exited 0; its installed
agent SHA-256 matches the bundle. Peer receipt 7081cae1ef944b568b1765a3f5146f8e
reports UPDATED and peer status confirms the same generation/revision. UI 85
tests, native builds, installer provenance and eight-file archive checks pass;
the verified update archive is 28,124,482 bytes. Ordinary inputworker/productlog
vet and git diff checks pass. Source-of-truth changes are this experiment and
the corrected g394 evidence; the private source protocol is unchanged.

Collection cb72a3576d65681001cb555cd03a2b3f at 23:24:29/30Z reports READY
services on both PCs and no fault/startup-failure event for the new revision.
No new capture session has been established, so this is installation evidence,
not a startup stability pass. A remains CONNECTING because B honored the earlier
explicit stop. Peer app-open receipt 707babd834fd188d18b8c4b72636872f reports
window_ready. B's explicit Connect remains pending from the user.

Physical input/held-input/native-stall, complete reconnect and release checks
were not repeated: the required peer connection is not established, and this
unit does not change native injection. Do not reuse the earlier g377 user
acceptance for this build. Ten issues remain open. Next work: finish the paired
reconnect and preserve any first startup failure before reinstalling; then
complete #50's synchronous parent authority/desktop audit, #59's callback
dependencies and #70's actual endpoint-change trial. No new implementation
unit was started after this round's ten-minute cutoff.
