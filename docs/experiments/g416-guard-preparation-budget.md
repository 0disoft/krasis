# g416: preserve the input caller's budget through guard preparation (#50)

The product press gate passed no context to crash guard journal preparation.
Both Process and PortableProcesses began an independent two-second budget even
when the input request had much less time remaining. A regression through the
production guardedProductInput with a real unresponsive child reproduced a
75ms request spending 2.000s in guard preparation. No input adapter was called.
This isolates guard IPC delay from network or native injection latency.

Process and PortableProcesses now expose PreparePressContext; the product guard
interface requires it and passes the press context. Pair admission, protocol
admission and acknowledgement consume the same remaining deadline. The existing
context-free entry points remain wrappers for lab callers. Preparation timeout
after admission retains exact-owner abort, failed-protocol and pair input fences;
an expired waiter that never admitted IPC does not abort the active owner.
Late acknowledgements cannot revive an expired operation or authorize input.

Only preparation inherits the press budget. CancelPreparedPress and Released
retain their existing independent bounded cleanup budgets and journal semantics;
the change cannot skip a key-up or erase uncertain held-input obligations merely
because the original injection context expired. The protected worker's existing
independent journal stays authoritative, with its ordinary guard remaining a
no-op. No input is detached, replayed, or injected by a new goroutine.

Verification: the old production test failed at 2.000s. The fixed real-child
test returns under its 500ms scheduling allowance for a 75ms deadline, performs
no injection and observes actual child exit. Deterministic pair tests verify an
exact 75ms total across contended admission plus acknowledgement, late-receipt
rejection, and fresh preparation after an unadmitted 50ms timeout. Crashguard,
residentagent, peer, application and labcli suites pass; crashguard/residentagent
vet passes. Existing partial/unknown-injection ledger tests remain green.
These are controlled IPC tests, not physical key, Winlogon or power acceptance.

## Issue and UI audit

Fresh remote issue inspection still lists Dashboard #1 (retained by request),
#4/#5/#6/#7/#15/#16/#20/#50/#59/#70. The first four require actual device/user
trials; #4 also requires an exact-revision CI-green kit, which a forbidden push
cannot supply. #16 still requires signing and complete install/remove acceptance;
#70 requires an actual endpoint change. #50/#59 retain remaining native/physical
proof requirements. No issue was closed from adapter tests or idle health.

Computer Use selected exactly one g415 product window. Its accessibility tree
shows CONNECTED and labeled controls, but the screenshot still displays the
Windows Security firewall permission prompt. No input was sent to that prompt,
no permission changed, and no screenshot or private desktop content was saved.
The existing user handoff remains pending under the Computer Use prohibition on
automating Windows security apps. This is not screenshot/keyboard/DPI acceptance.

No public wire/API/DB schema, dependency, runner, firewall, signing, repository
hygiene, branch/worktree, remote ref, push or issue-comment change. Generation
advances only to support the authorized distinct paired bundle. Race detection
is unavailable with CGO disabled; hosted CI remains unrun because push is forbidden.
Bundle/application evidence is appended after execution.

## Bundle and paired application

UI 85 tests, native builds, installer provenance, workspace archive test and
eight-file package verification pass. Offline verify accepts g416 revision
eb6047375746698ecba7635239f59b53c9c75673, 28,177,246 bytes.

A's one automatic upgrade began at 02:57:31Z on 2026-10-21 and completed at
02:57:42.1438788Z, result 0, recovery not_needed. No manual stop or retry was
needed. Installed-agent SHA-256 matches the bundle. Fresh doctor reports healthy
CONNECTED, matched g416 UI/resident/installed revisions, verified launch record,
running service and completed update-stage cleanup.

B update 3b42f6e93f6444efc10937ca84b0d605 reports UPDATED at the same revision.
B installation completed at 02:58:11.1245763Z, result 0, recovery not_needed.
Fresh peer status reports ONLINE at g416. Collection
eb4662b033f27dadbd44b72b3dddf933 at 02:58:52Z contains 28 A / 29 B current-build
events, CONNECTED on both, service/input READY and no current-build fault or
startup-failed event. This short connected interval is not physical held-input,
native injection latency or long-duration soak acceptance.

The remaining native callback route still performs the required immediate
Scope.Do -> desktopBinding.active -> UOI_IO check and synchronous first-input
cursor restoration on its owner thread. Adapter deadlines cannot prove those
native calls or actual held-key cleanup on Winlogon. No additional speculative
native changes were made. Full installation/signing, attended device/power and
endpoint-change trials, the Windows Security UI handoff and exact-revision CI
remain outstanding. No open PR was found, no issue was closed, and no new work
unit was started during packaging/deployment verification.
