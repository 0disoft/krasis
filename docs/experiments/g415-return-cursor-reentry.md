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

## Completed verification and paired application

Wininput vet with only the existing unsafe-pointer analyzer disabled passes;
the full analyzer result remains the eight warnings described above. UI 85
tests, native builds, installer provenance, workspace archive test and eight-file
bundle verification pass. Offline verify accepts g415 source revision
ea0ff90292c3873b01cc7258c4805f2d55e8f289, 28,182,549 bytes.

A's automatic upgrade began at 02:48:06Z on 2026-10-21 and completed at
02:48:18.4841518Z, result 0, recovery not_needed. No manual stop or retry was
needed; installed-agent SHA-256 matches the bundle. Fresh doctor reports healthy
CONNECTED, matching g415 UI/resident/installed revisions, verified launch record,
running service and completed update cleanup.

B update 82b01f00ca8027deee06c281b99fae8a reports UPDATED at that same revision.
B's installation completed at 02:48:54.8474412Z, result 0, recovery not_needed.
Fresh peer status reports ONLINE at g415. Collection
79a7b2f2817da67b7948a653f51d6983 at 02:49:16Z has 23 A / 24 B current-build
events, CONNECTED on both, service/input READY and no current-build fault or
startup-failed event. This short observation does not establish hardware input,
lock/sleep, native latency or soak acceptance.

## Remaining scope

Current remote issues remain #1 (Dashboard retained by request), #4/#5/#6/#7/
#15/#16/#20/#50/#59/#70. None has new complete acceptance evidence from this
unit, so none was closed or commented on. The worktree is kept on local main;
no push or remote ref update was performed.

The remaining #59 audit must distinguish fixed local lock/reentry hazards from
required immediate UOI_IO/cursor/native window calls. Do not remove those checks,
move hook work to a detached goroutine, or call a bounded adapter fixture proof
of native/hardware retirement. #16 still needs exact owner evidence in the
no-API path and signing/install-remove acceptance. #6/#70 need attended lifecycle
and actual endpoint-change trials; the earlier availability question remains
unanswered. The Windows Security UI handoff and exact-revision hosted CI also
remain pending. No further source unit was started during deployment checks.
