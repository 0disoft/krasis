# g413: bound proof admission before source desktop work (#50/#59)

The source worker's full authorization callback still queried service/agent
proofs directly. It is supplied to NewProtectedSourceHook, its boundary guard,
and the window-free source command desktop. This is distinct from the live
UOI_IO query performed on the actual hook/desktop owner.

A regression through the real NewProtectedSourceHook.WithSourceCommandDesktop
path took two seconds despite a 50ms owner deadline. The controlled proof always
rejects, so the test never opens a desktop or injects input. Both service and
agent stalls are exercised. The callback now uses the existing full source
command query owner with one shared one-second limit and earlier owner
cancellation/deadline. It cannot grant authority from a late result or close a
native query's handles early. Presence-only results remain insufficient.

Only read-only proof queries use the Channel query worker. Desktop binding,
current-desktop validation, cursor work, input, hook/window ownership and native
thread retirement keep their original thread affinity and security checks.
No new cached proof, detached desktop operation, replay or additional native
query capacity is introduced. Remaining native desktop/cursor queries still
need their own evidence; this alone does not complete #50/#59.

## UI evidence boundary

The Windows Computer Use helper returned exactly one Krasis g412 window and an
accessibility tree showing CONNECTED, labeled primary actions and named regions.
Its screenshot showed a Windows Security firewall permission dialog over another
window rather than a reviewable Krasis screen. No screenshot or private desktop
content was saved or committed. Do not infer layout, high-DPI or keyboard
acceptance from the accessibility tree. UI input was not attempted; the pending
user handoff is required because Computer Use prohibits automating Windows
security apps/permission changes. The attended physical matrix question also
remains pending.

Verification/application results are appended after execution. No public control
API/DB, dependency, runner, firewall, signing or repository hygiene change. No
push, comment, remote ref, new branch or worktree. Race detection is unavailable
with CGO=0; local checks do not stand in for physical held-input/Winlogon, power,
home-pair, endpoint-change, accessibility or signed-release acceptance.

## Verification and paired application

Inputworker/agent/residentagent suites and inputworker/residentagent vet pass.
The production desktop-entry regression now returns at its 50ms budget for both
stalled proof owners. UI 85 tests, native builds, installer provenance, archive
test and eight-file bundle verification pass. Offline verify accepts g413 revision
f4d7ce6b6afa701930c6700a88bd6585114362ad, 28,208,467 bytes. Source is f4d7ce6.
No hosted CI/push, CGO race detection or extra resident-conflicting native smoke.

A's automatic installation at 02:18:24Z failed at replace/directory_retire_move,
Win32 5, with the old installation restored. Therefore g412's observation fence
has not eliminated every real folder-lock cause. The current UI was closed,
safe-stop requested, and the observed resident joined before one further install.
That install completed at 02:20:51Z, result 0, recovery not_needed; installed-agent
SHA-256 matches the bundle. UI and Connect B restored the original session intent.
This does not establish the original blocking handle's owner.

B update d7754e45459e176678235d65f5617d04 reports UPDATED at g413. B installation
at 02:21:41Z reports complete/result 0, recovery not_needed. Collection
d823615074889bee753be53e5e15d2c2 at 02:22:05Z/06Z contains 53 A / 24 B current-build
events, CONNECTED on both, service/input READY, and no current-build fault or
startup-failed event. A's fresh doctor independently reports healthy CONNECTED,
matching UI/resident/installed revisions, verified launch record and completed
stage cleanup. These event checks do not erase A's installation failure.

The external receipt-monitor script had a separate observation bug: this
PowerShell converts JSON ISO timestamps to System.DateTime; DateTimeOffset.Parse
then coerces that object through a string and assumes the local +09 offset.
The observed value cast directly to DateTimeOffset preserves UTC. Consequently,
the poll's failure to print a fresh receipt is not evidence of delayed installation.
Direct inspection confirmed each terminal receipt; no process was restarted just
because that observation expired. Use a direct DateTimeOffset cast for future
receipt comparisons. This was tooling only, not an application timestamp change.

## Remaining evidence and next run

Remote state has no open PRs, Dashboard #1 retained by request, and incomplete
#4/#5/#6/#7/#15/#16/#20/#50/#59/#70. No issue comments or unsupported closures.
Both the Windows Security handoff and attended physical-matrix availability
question remain unanswered. The g412 accessibility tree is partial UI evidence,
not screenshot/keyboard/high-DPI acceptance or a g413 UI review.

For #16, installSCM.stop already pins the exact service process and waits for its
kernel exit. Startup.Available checks the resident's named singleton rather than
its exact process exit. Investigate that boundary with a controlled exiting-child
fixture before changing it; no contemporaneous handle owner was identified here.
For #50/#59, read-only full proof admission is now bounded, but actual desktop,
cursor and native cleanup behavior still needs the remaining audit/physical
evidence. Preserve immediate desktop and first-click semantics. No new work unit
was started past the ten-minute cutoff; the completed source/application record
is local main only.
