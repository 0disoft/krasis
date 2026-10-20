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
