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

## Paired g377 acceptance

Both PCs run generation 377, revision
`c0e86f3cb022743fabd0987216f5f211b5786d7d`. Package verification accepted
27,843,581 bytes. Frontend's 85 tests, native build, installer provenance and
eight-file update archive checks passed. B's receipt
`8fa9ce883189cfc1026f547cc3216634` reports UPDATED at that exact revision.
The SEND command returned agent_unavailable rather than a receipt, but a
read-only status recovered the existing completed receipt. No second SEND was
issued. That lost response is not treated as proof of an agent crash.

A's installer returned 0; the installed agent SHA-256 matches the bundle.
Doctor reports healthy/CONNECTED with matching CLI, UI, running/installed
agent and recovery record. The paired 2026-10-20T19:49:29Z/30Z collection
shows READY input health on A and B. Both connected at 19:49:21Z. Neither
current-build collection contains a fault/session_failed event. This short
window does not establish physical mouse/held-input or long-term stability.

A has a separate startup update-stage cleanup warning: failed, removed 0.
B reports cleanup completed, removed 0. No folders were manually deleted and
no cleanup success was fabricated. Prune intentionally runs before management
endpoints are exposed and refuses managers with live network workers; it must
not be bypassed using another manager during the active session. Inspect the
protected-directory enumeration and original prune error in a later bounded
maintenance step. Input/service health remains READY despite this warning.

Ten issues remain open, including #59. Resume the remaining callback work
without moving a stale desktop authorization or a late first click across
its required boundary. #50 remains the separate non-cancelable injection
lifetime task. Hardware/report/signing gates #4/#5/#6/#7/#15/#16/#20 and
endpoint hardware acceptance #70 remain unproven.

No third task was started after ten minutes. All changes are local main
commits in the existing checkout; no push, new branch/worktree or issue comment.
The dependency dashboard remains untouched.
