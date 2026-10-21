# g410: reject geometry results after session retirement (#50/#59)

The production geometry guard checked its permanent invalidation bit and session
context only before CurrentDisplayGeometry. Two controlled tests failed: a read
that canceled its session still returned success, and a read returning the old
geometry after a concurrent caller invalidated it also returned success. The
guard participates in source boundary and destination entry validation.

After a matching native result, recheck permanent invalidation and cancellation.
Changed/unsupported geometry still invalidates exactly once and invokes the same
session-stop callback. No geometry result is cached, no input is replayed, and
the native read stays on the original caller. This fixes stale admission rather
than claiming to cancel a Windows native query.

The callback audit confirms capture scopes use a fixed desktop binding whose
Scope.Do checks live UOI_IO, without SCM/WTS on every mouse move. Immediate native
desktop checks and visible return-cursor shield hiding remain necessary; moving
them to detached work would not preserve desktop/first-click correctness. No
speculative change was made to these native boundaries.

The two geometry regressions fail before the fix. Focused geometry tests, relevant
package verification, build and paired application are recorded below after
execution. User confirmation of g377 ordinary input is not current-build held-key,
power, secure desktop, home-pair, DHCP, accessibility or signed-beta evidence.
No API/DB, dependency, runner, repository hygiene, firewall or signing change.
No push, issue comment, remote ref edit, additional branch or worktree.

## Verification and paired application

Residentagent/application/agent suites and residentagent vet pass, including both
new geometry regressions. UI 85 tests, native builds, installer provenance,
archive test and eight-file package checks pass. Offline verify accepts g410
revision 2f6aa989f595f1494bd140549756e0b9cac57797, 28,261,734 bytes. Race detection
was not run with CGO disabled; hosted CI was not run because push is forbidden.

A's automatic installation at 01:54:22Z failed at replace/directory_retire_move,
Win32 5, with the previous installation restored. A direct retry after safe-stop
at 01:55:57Z failed at the same operation with Win32 32, also restored. A process
snapshot showed a new portable resident after that stop. The healthy-session UI
was then closed, safe-stop was requested again, and the observed resident was
joined before retrying. Installation completed at 01:56:48Z, result 0, recovery
not_needed; installed-agent SHA-256 matches the bundle. The exact handle owner
causing either failure is not established by this intervention. Do not add blind
installer retries or weaken the image pin/sharing policy from this observation.

The UI was restored and Connect B explicitly restored the connection intent that
safe-stop had cleared. B update e50ba3fdbddeb2b8d4f95b596f0724d1 reports UPDATED;
B installation completed at 01:57:33Z, result 0, recovery not_needed. Collection
69ce307ae646b6942b1b51e863cb5f65 at 01:57:59Z has 76 A / 28 B current-build events,
CONNECTED on both, service/input READY on both, and no current-build fault or
startup-failed event. These event results do not erase A's installer failures.
A doctor independently reports healthy CONNECTED and matching UI/resident/
installed revisions, verified launch record and completed update cleanup.

## Completion audit / next run

Current GitHub state has no open PRs, intentionally retained Dashboard #1, and
ten incomplete issues. CurrentUser/My and LocalMachine/My contain zero unexpired
code-signing certificates with private keys. Remote signing service availability
is unknown; no secret or certificate material was requested or exported.

| Issue | Missing completion evidence |
| --- | --- |
| #4 | Same-revision study-pair queue/disposition/admission matrix and checker result; current builds have no exact-revision hosted CI evidence. |
| #5 | Home-pair identity/inventory, baseline and bidirectional device results. Earlier pair-identity question remains unanswered. |
| #6 | Attended physical termination/held-input, network, lock, sleep, topology and uncertain-injection matrix; new availability question is pending. |
| #7 | Current hardware/DPI/privilege/polling-rate compatibility, bounded latency and soak evidence. |
| #15 | Current screenshot/keyboard/accessibility/high-DPI review and published exact-revision CI; publication remains forbidden. |
| #16 | Signing plus full current-build install/remove/cleanup/firewall device acceptance; investigate observed folder-move failures without guessing the handle owner. |
| #20 | Umbrella depends on the unresolved linked hardware/product/release criteria. |
| #50 | Remaining native call-graph/lifetime audit and actual held-input recovery, including Winlogon; isolated-child fixtures are supporting evidence. |
| #59 | Required native desktop/cursor callback work cannot be treated as nonblocking; physical failure/retirement evidence remains missing. |
| #70 | Real authenticated two-PC endpoint/address change; unchanged-IP DHCP is not that test. |

No issue was closed from unit tests or idle connection health alone. Next useful
work is native/physical evidence and installation lock-owner attribution, rather
than speculative removal of desktop checks. No new implementation unit was
started after this round's ten-minute cutoff. Only local main exists; completed
source is committed independently, and paired-application evidence is separate.
