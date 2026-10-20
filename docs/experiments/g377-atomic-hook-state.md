# g377: atomic hook state reads and capture-owner publication (#59 progress)

SourceSession's active bit is now atomic, published inside the existing state
transactions. Active() no longer obtains stateMu, so physical mouse movement
and button eligibility cannot queue behind a state writer merely to read the
suppression bit. Terminal remains the independent, final fence. Generation,
held-input/revision checks, yield completion and enqueue transactions retain
their existing ownership and order; this is not a cached authorization result.

Source wheel capture now publishes its one current native owner through an
atomic pointer. Active/OwnsWheel perform no runMu wait, attach permits exactly
one owner and a stale detach cannot clear its successor. Foreground ownership
is still checked immediately; arbitrary NIC/input/desktop authority is not
inferred from these atomics.

The duplicate serializedBoundaryObserver was removed. Both raw and low-level
physical routes already hold the same SourceSession.physicalMu across boundary
observation. Its concurrency test now exercises the actual routing gate and
still proves eight concurrent producers cannot call the detector concurrently.

Passed wininput, agent, inputworker and residentagent suites. Focused tests also
prove that a Windows physical mouse callback completes while stateMu is held,
that a terminal source passes local input despite an old active bit, that 32
concurrent capture candidates admit only one native owner, and that stale owner
detach cannot remove a replacement or retain wheel suppression after retirement.
Existing queue/discrete-input, activation revision, local-return, capture refresh
and held-input cleanup tests passed. One accidental duplicate test process was
stopped; only the completed verification run and focused new tests are accepted.

#59 remains open. Required immediate desktop/foreground checks, synchronous
return-cursor hide, physical ledger/queue gates and optional pipeline observer
callbacks are still present. Moving cursor hide to PostMessage alone would not
prove that the first click reaches the intended application. Stale desktop
activity must not be cached to avoid a native query. A future change must
preserve these behaviors while bounding the remaining callback work.

No product control API, DB, dependency, diagnostics schema, runner or firewall
change. No raw keys/positions/desktop names are added to logs. No issue comment
or push. Device application and its verification are recorded below.
