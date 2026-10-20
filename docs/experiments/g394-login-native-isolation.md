# g394: retain login recovery outside native input (#50)

The before-login connection now retains its pressed journal, network lifetime
and Winlogon recovery scope while a distinct native child executes input. The
existing unbuffered login mailbox still serializes commands and preserves
actual/partial/unknown outcomes. On termination, the callback is joined, then
the native child's full retirement is proved before journaled key-up. Missing
retirement preserves Remaining and ErrRecovery, even with an empty journal.

This is a distinct fixed `--native-login-worker` mode, not the ordinary agent
bootstrap. Before login there is no user agent to authenticate. Its capability
is constructed only after the real SCM login pipe handshake and activation.
The child independently validates the protected installation, actual SYSTEM/
UIAccess console token, service/SCM pipe, exact protected live parent, activated
session/profile binding and current administrator-owned login grant. Input stays
on the fixed active Winlogon desktop. The profile hash, inherited handle values
or local RPC binding alone never grant authority. No peer-selected PID, command,
path, environment or desktop is accepted. Source TLS/trust remains in the
existing approved login connection; the child has no network listener or keys.

The two inherited identity references are consumed once and inheritance is
cleared. Login jobs now permit precisely the connection worker and its native
child. Service retirement requires the exact authenticated worker's final
recovery receipt, which follows native-child exit and IPC cleanup, plus the
service's own exact parent-exit wait. A missing receipt is terminally fenced,
with job/image/process ownership retained until service exit. An accounting
count of zero cannot substitute for this proof. Ordinary input arms this fence
before bootstrap transfer; login input arms it immediately before activation.
Only validated final recovery/clean receipts can satisfy it.
Ordinary and login native transports share framing and retirement, with separate
authenticators and fixed desktop policies. Explicit cursor-placement probes
remain probes across the local client instead of dropping that interface.

## Validation

A real hidden native stand-in acknowledges a simulated down and then ignores
cancellation during Move. The actual login mailbox/owner joins that child's
bounded retirement and releases the held journal only after exact exit. Tests
also preserve held/empty uncertain retirement without key-up, reject malformed
or non-activation bootstraps, duplicate/missing references, uninstalled native
login entry and absent control ownership. Native job information confirms the
two-process kill-on-close bound. These fixtures inject no native keys/buttons
and do not claim a physical Winlogon test.

The first full suite reproduced the g393 accounting gap: Close returned success
while the second real process handle returned WAIT_TIMEOUT. The updated native
test requires an exact native exit before recording a final receipt. A separate
test proves that zero accounting and parent exit cannot revive missing receipt
ownership or retry poisoned handles. This matches the documented
[process-exit synchronization boundary](https://learn.microsoft.com/en-us/windows/win32/procthread/terminating-a-process).

Inputworker, residentagent, application, peer, input-command and agent-command
test/build checks and ordinary vet pass after the fix. Packaged application is
recorded below when complete. No logout, lock,
suspend, reboot or secure-desktop operation is initiated for this validation.

## Remaining acceptance

The journal owners still contain synchronous authority/desktop checks outside
the isolated injection child. Audit their real input/cleanup call graph before
closing #50; a successful cursor smoke does not establish every failure path.
Physical input/fault, home-pair, endpoint-change, UI/accessibility and signed
release gates keep their own requirements. No push, issue comment, external
API, DB, runner, dependency, firewall or signing change is part of this unit.

## Packaged application and observed startup failure

Both PCs applied generation 394, revision
2213b731fa8113f999470dda229372902546da92. UI's 85 tests, native builds,
installer provenance and eight-file archive checks passed. Offline verify
accepted 28,270,809 bytes. A installer exited 0 and its installed agent SHA-256
matched. The actual protected agent's ordinary service/native-child cursor
read/same-point placement/shutdown smoke passed. Only the session-0 service
remained afterwards; no console input processes remained. This is ordinary
installed-path evidence and does not exercise the before-login Winlogon path.

A's first source startup failed at 2026-10-20T23:09:47Z, stage worker_ready,
native error unknown, with fault_remote_eof/session_failed. Automatic recovery
restarted A at 23:09:57Z, replacing the first update receipt
9f9e5fe792234fe8d84fb4bfee97058b before B had changed from g393. Fresh status
confirmed IDLE/peer g393 before a new send was created. B receipt
22765218a4ebcef70a7cdf0ab4bb0f52 reports UPDATED/g394. Its replacement then
ended A's connection; A was explicitly reconnected at 23:11:51Z. No Disconnect
command was sent during replacement. The initial startup failure is unexplained
and is not dismissed as a successful or fault-free stability trial.

Doctor now reports healthy/CONNECTED, matching CLI/UI/resident/installed builds,
protected input CONFIGURED and completed stage cleanup. Peer status is ONLINE
at the same g394 revision. Collection bcb106e38fc134dbed439687dc62be06 at
23:12:21Z has 127 A / 60 B current-build events, both READY, and no new fault or
triggered session-end detail after the explicit reconnect. This short window
does not establish physical input, soak, DHCP change or before-login recovery.

Follow-up paired collection 871f1c56129da71bf464e8fd7c1c6e23 establishes that
B's first failure at 23:09:47.5983677Z was still revision 5cace49 (g393), before
its update. B selected source-hook as the triggering component. A g394 then
observed peer EOF at 23:09:47.7056422Z; its service source summary records
command not_started, reader eof, joined capture/sender/interrupt and failed
retirement delivery. These are not two independent g394 startup failures.
The original B service summary was replaced by its subsequent installation,
so the first native/service cause cannot be recovered from the current summary.

Next priority is the observed installed source worker_ready startup failure and
its automatic agent replacement, before speculative additional authority work.
Retain the first collection 08960d16ba365607fc7321cf690749f9 and later
8d4b5790393bd4de198fb11c625e0ce8 for that investigation. All ten issues remain
open. No new implementation unit started after the ten-minute cutoff; completed
source and rollout evidence are local main commits with no push.
