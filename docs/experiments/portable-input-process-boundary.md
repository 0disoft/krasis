# Portable input process boundary (#50), generation 389

## Product ownership

The ordinary productRunner route now uses distinct resident R, input/cursor I
and recovery guardian G lifetimes. The installed privileged input route remains
separate and continues using its authenticated service workers. The ordinary
route does not fall back to in-process SendInput or native cursor I/O on failure.

Private agent modes consume inherited stdio and closed process-handle bootstrap
references. I and G each own a fixed Default desktop on a dedicated OS thread.
Desktop activity is checked immediately before native work; no peer-selected
PID, executable, desktop, token, service identity or network endpoint is accepted.
This adds no SYSTEM authority or new network listener.

R's existing guardedProductInput orders journal prepare/cancel/released on G
against input/cursor operations on I. The parent InputAdapter retains return and
entry transaction state; injection, cursor read/write, placement/readback and
entry snapshot execute through the child. Parent-side geometry/admission work
and source hooks are separate boundaries and are not claimed isolated here.

Input replies retain sequence/binding checks, partial/not-applied/unknown
certainty, conservative submission fencing and no replay. The sole IPC admission
gate shares the original caller budget across both children. A failed input
owner is never replaced before full retirement.

## Recovery and retirement

G inherits pinned references to exact R/I process objects. On R death, it stops I
and requires exact kernel exit before releasing the held journal. EOF or a
successful termination request alone never proves retirement. A denied request
waits for actual exit; failed/unexpected native waits permanently stop polling
that handle. Missing proof reports remaining input and no successful recovery.

Normal R shutdown rejects new admission, joins the admitted command, fully
retires I (including owned native/IPC cleanup), then disarms/joins G. One
ExitPending.Done covers both children and every outstanding native operation.
Live Done reports either child's exit but cannot authorize replacement. Repeated
Close observes the same owner; both children's error causes remain visible.
Startup/client/adapter failures preserve this same cleanup receipt.

## Validation

Passed: crashguard, inputworker, residentagent and wininput suites; agent command
compilation; vet on agent/crashguard/inputworker/residentagent; whitespace checks.

Real hidden Windows subprocess tests cover an acknowledged simulated down,
resident death while input ignores cancellation/EOF, exact input exit before
release, cooperative shutdown, repeated Close and denied termination. Pair RPC
fixtures cover lost ACK, uncooperative input and partial injection while the
independent journal survives. Synthetic tests cover two independently pending
children, delayed native cleanup, no duplicated native tasks, shared deadlines,
malformed/EOF/disarm failure, empty-journal fencing and poisoned handle rejection.
These fixtures inject no native key/button input.

A locally compiled real agent passed Default-desktop heartbeat, cursor read,
same-point cursor placement/entry snapshot, and full paired shutdown. This is
local cursor smoke, not physical keyboard/wheel/drag or laptop fault acceptance.
Race detection was skipped because CGO/compiler support is unavailable. No
hosted CI was invoked.

## Packaged candidate and paired application

Both PCs applied generation 389, revision
d8e5c26a72444acf25b9a6a41b48e7f143598882. UI's 85 tests, native builds,
installer provenance and eight-file archive checks passed. Offline verify
accepted 28,067,185 bytes. B receipt 5448e930e689aaaee0e4c87f66d7b1e7 reports
UPDATED. A installer exited 0 and installed agent SHA-256 matched the bundle.
No Disconnect command was sent during replacement.

The real packaged agent also passed the same ordinary Default-desktop cursor
smoke and paired shutdown. This directly exercises its private child modes;
it does not inject physical keys/buttons or establish fault-matrix acceptance.

Doctor reports healthy/CONNECTED with matched UI, resident and installed build,
protected input CONFIGURED and completed stage cleanup. Collection
ba41ce8f718bd6989914a151a06df388 at 2026-10-20T21:58:15Z/17Z reports READY on
both PCs. Its 30 A / 58 B current-build events contain no fault_ or session_failed
and each contains installed_source_capture_protected_ready. B recorded a network
acquire timeout/RECONNECTING while A was being replaced, followed by CONNECTED;
that deployment interruption is not suppressed or counted as a physical trial.

This is short-window startup/runtime evidence. Both installed PCs select
protected service input, so their healthy state does not establish ordinary
physical-input recovery. No hardware, accessibility, endpoint-change or signing
gate is closed by this rollout. No push or issue comments were made.

## Remaining acceptance

- Obtain ordinary-route physical input/fault evidence; current A/B installations
  select protected service input, so their healthy state cannot prove this route.
- Continue the full #50 native-boundary audit, including parent geometry/admission
  and installed worker cleanup, before declaring the broad issue complete.
- #59 callback work and #70 physical endpoint change remain separate. Hardware,
  accessibility, installer and signed-release gates (#4/#5/#6/#7/#15/#16/#20)
  still require their own evidence.

All ten issues remain open. No issue comments, push, extra branch/worktree, public
API, DB, runner, dependency, firewall or signing changes are part of this unit.
