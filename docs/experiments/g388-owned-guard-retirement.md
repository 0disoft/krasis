# g388: bound guard shutdown while native Kill/Close remain blocked (#50)

g387 bounded protocol admission, but invoke still synchronously called native
abort after its two-second timer. Process.Close also called stdin.Close and
abort synchronously before its final exit timer. A blocked native Kill/Close
could therefore defeat both caller budgets and retain runtime/session cleanup.

One Process now owns one protocol call, one retirement coordinator, one Kill
attempt and one Close attempt per pipe. Those tasks retain their original
resources until they return; they cannot accumulate on repeated Close or later
input calls. Kill and pipe closure start independently so one blocked native
close cannot prevent termination from being requested. Abort fences admission
before starting any asynchronous task. Late replies cannot authorize input.
No actual native injection is detached or replayed.

invoke returns ErrIPCTimeout within its original IPC budget without joining a
blocked Kill/Close. Close returns within two IPC budgets (normally four seconds).
It attempts Disarm within a graceful interval, then requests bounded-owner abort
if the guard has not exited. Repeated/concurrent Close calls share the same
retirement and stable final result. Termination/pipe/protocol failures remain
errors; completion never certifies successful key-up or clears a fault latch.

Process.Done retains the existing process-wait notification for the live guard
monitor. ExitPending.Done now represents full retirement: exact process wait,
all protocol work and all requested Kill/pipe Close tasks joined. Child exit
alone cannot release the resident agent's existing replacement/upgrade fence
while old native operations still own resources. ADR 0036 records this stronger
contract without changing the public guard_exit_pending status code.

## Validation

Crashguard, residentagent and labcli suites passed; crashguard/residentagent vet
passed. Controlled-time tests block Kill and both pipe closes, issue eight
concurrent repeated Close requests and prove bounded caller return, one native
attempt per resource and one shared completion. Tests separate real process
exit from full native cleanup, and keep retirement pending until a stalled
protocol operation actually returns. Original failures survive late completion.

A real hidden Windows test child ignores parent EOF. Its Kill is delayed while
actual request/stdio handling times out. The caller returns before Kill is
released, reports ExitPending for the still-live child, then joins actual
termination and owned cleanup after release. Existing real hung-helper and
parent-EOF recovery tests also passed. No input is injected by the new child.

The race detector remains unavailable because CGO/compiler support is absent.
No hosted CI, physical lock/suspend/network fault matrix, or signed release was
run. Paired application/source readiness must still be recorded after rollout.

## Remaining work

#50 still includes portable in-process native injection. product_runner's
worker==nil branch constructs NewInputAdapterWithReturnRearmDistance with no
external injection port; Press/Release/Move/Wheel then call native code on the
resident runtime's thread. InputAdapter also performs synchronous cursor
transactions/readback. Moving only SendInput would leave those other native
calls able to retain runtime ownership. The next implementation needs a real
owned process boundary covering the complete portable input/cursor contract,
with input order, partial/unknown certainty, parent death and pressed-state
recovery retained. Protected worker authentication must not be weakened or
reused to accept an ordinary portable process as a SYSTEM service.

#59's callback dependencies and #70's physical endpoint trial remain separate.
Hardware/accessibility/signing requirements of #4/#5/#6/#7/#15/#16/#20 remain.
No public API, DB, dependency, CI runner, firewall or signing changes. Only the
internal retirement contract, source/tests, generation and this record change.
Local main only; no push, GitHub comment, extra branch or worktree.
