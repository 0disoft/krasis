# g415: retire stale return-cursor presentation across native reentry

While auditing #59, the production return-cursor show path was found to check
its generation only before querying the native cursor. A controlled regression
reenters the production beforeInput method during the cursor query or window
show, with unchanged coordinates. Both cases fail on g414: the first still shows
a canceled shield; the second leaves the native window visible while its ledger
says hidden. This is deterministic adapter evidence, not a claim that the
reported physical stutter was caused by this sequence.

Show now rechecks generation after the cursor query and after the native show.
A canceled show that completes after the nested hide performs a final hide on
the same window. Hide publishes its in-progress state before calling Windows,
preventing another hook callback from recursively hiding the same window.
Native cleanup failures restore the visible ledger and retain the existing
cursor-restore fault. The position mutex no longer spans native query/window
calls or presentation posting, so native reentry cannot wait on that same lock.

The work stays on the existing native owner thread. It adds no detached input,
cached desktop authorization, replay, native query worker or new resource owner.
Current-desktop checks, immediate first-input hiding, error latching and native
destruction ownership remain in place. The bundle generation advances solely to
permit the authorized distinct paired deployment.

Verification: both old-code stale-presentation cases reproduced failure; the
fixed return-cursor tests cover query/show invalidation, hide reentry and failed
late-hide cleanup. Wininput/inputdesktop/residentagent/productpanel suites pass.
Full wininput vet reports eight unsafe.Pointer warnings in unchanged Win32
callback trampolines; this is not a clean full vet result. Additional verification
and paired application results follow after execution. Race detection remains
unavailable with CGO disabled; hosted CI remains unrun because push is forbidden.

The no-API startup audit did not identify the previous installation's blocking
handle. The process loader outlives resident Run's lease; a newly discovered PID
is not authenticated exit evidence. No speculative process selection or pin
weakening was added. #16 remains incomplete. #50/#59 still require their remaining
native call-graph/physical evidence; this cursor correction alone closes neither.
The g377 user input confirmation remains specific to that build.

No public API/DB schema, dependency, runner, firewall, signing, repository hygiene,
branch/worktree, remote ref, push or issue-comment change.
