# g372: native activation inspection no longer owns the physical hook gate

`SourceSession.Activate` previously called the native pressed-state inspector
while holding `physicalMu`. The low-level keyboard/button path takes that same
mutex before updating its physical ledger. A slow native inspection could
therefore block physical hook progress even though activation is performed on
a separate command owner. This is a reproduced dependency, not proof that the
g370 EOF was caused by it.

Activation now snapshots the inspector and physical revision under a short
lock, runs the native inspection without that lock, and reacquires the gate
to validate the result before granting a generation. Physical down/up, repeats,
capture pause/resume and inspector replacement invalidate the snapshot. A
complete local down/up during inspection also rejects the stale result, even
if the held ledger is empty again. A counter at saturation cannot wrap into
validity. Retirement, pending local return and active-generation checks remain
under their existing fences. Native authorization and pressed-state checks
are retained; no delayed physical event is replayed as remote input.

The controlled regression holds native inspection open while delivering local
keys/buttons, a full local down/up, owner pause/resume and retirement. The old
Activate implementation fails the hook nonblocking assertions; the new code
passes them and rejects changed state. Stable, unchanged inspection still
activates. Full wininput, inputworker and agent suites pass.

This removes one concrete wait from #59. Callback desktop activity checks,
cursor presentation, observer contracts and the remaining Go lock scopes are
still part of its audit; #59 is not declared complete by this single change.
The native injection lifetime boundary in #50 and hardware/release gates also
remain open. No public API, DB, dependency or CI runner change; no issue
comments or Git push. Paired installation evidence follows verification.

## Paired installation

Both PCs run generation 372 at revision
`6836ce474b7de89a415cbe6c6bac986499a2c9b8`. B transfer
`8c11564ce0f58d8cc04d081282665ad3` reports UPDATED. Its earlier DELIVERED
observation was nonterminal; the same transfer completed without resending.
Offline verification accepted the 27,883,543-byte bundle. Native packaging,
installer provenance, UI tests and the eight-file update-package check passed.
A's installer returned 0 and its installed agent SHA-256 matches the bundle.
The CLI, UI, running/installed agents and recovery record match that revision.

Collections at 18:38:08Z and 18:38:48Z on 2026-10-20 show input health and
service READY on both PCs, with no new g372 fault/session failure. The benign
`installed_input_desktop_default` event is a desktop observation, not a fault.
A doctor is healthy/CONNECTED and B is ONLINE. This short observation does
not establish long-term stability, polling-rate compatibility or the physical
fault matrix. Inputdesktop, residentagent and localapi suites also passed.

Clarification for the remaining #59 audit: the protected capture binding does
not repeat full SCM/WTS authorization in each low-level callback. Its
`Scope.Do` checks current desktop activity on the owned thread. The outstanding
native callback work includes that desktop query and synchronous return-cursor
window hiding, as well as reviewing observer/Go-lock scopes. Removing a desktop
check or deferring a hit-testable cursor shield needs its own safety argument;
this change does neither. All 14 remaining issues are still open.

No public API, DB, dependency, runner or source-of-truth product contract was
changed. Only local main commits were made; no issue comments or Git push.
