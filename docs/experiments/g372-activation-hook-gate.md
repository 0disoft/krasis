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
