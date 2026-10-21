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
