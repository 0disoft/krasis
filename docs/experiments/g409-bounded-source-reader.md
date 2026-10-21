# g409: bounded source reader and watchdog authorization (#50/#59)

The read-side command and presence watchdog used direct service/agent native
queries. A controlled delayed-native regression reproduced a 50ms caller taking
two seconds and returning success. Both owners now receive one shared one-second
budget (an earlier worker cancellation/deadline wins), with independent full and
presence lanes under the existing process-wide query capacity. No result is
cached; presence still cannot authorize commands requiring a full proof.

The capture bootstrap transfers the service proof and control pipe together to
a Channel owner after startup cancellation is stopped. Shutdown interrupts the
control reader by deadline without closing a query's pipe. The agent pipe also
transfers to its Channel instead of retaining an unconditional bootstrap Close.
Expired native queries keep handles/capacity until actual return; pending service
cleanup is reported as uncertain rather than successful. Capture/sender/native
thread joins and the protected hook's desktop-affine checks remain intact.

Verification: the old direct-call implementation failed the controlled deadline
test. Focused tests cover stalled service/agent full and presence queries, 50ms
expiry, exactly-once handle retirement, and one shared budget across both owners.
The existing query capacity, late-success, heartbeat independence, and native
resource-retirement tests are retained. Broader package checks/application status
are recorded below after execution. Race detection is unavailable with CGO=0.

This is supporting implementation evidence for #50/#59, not completion of their
native callback/physical acceptance gates. User-confirmed round trips, click,
keyboard, drag and both scroll directions belong to g377; no newer-build physical
acceptance is inferred. Dashboard #1 remains open. No public API/DB, dependency,
runner, signing, firewall or remote Git ref change.

## Verified paired application

Inputworker/application/peer/residentagent/productpanel checks and inputworker vet
pass. The later shared-budget regression also passes. UI 85 tests, native build,
installer provenance, archive test and eight-file bundle verification pass.
Offline verification accepted g409 revision
e215d645e1cdd17115d5d7e3f0f3c3fee9ad98ed, 28,261,174 bytes.

A installed at 01:45:43Z and B at 01:46:35Z: both complete/result 0,
recovery not_needed. A installed-agent SHA-256 matches the bundle. B update
e2ac5899986ab2a25ebb60298a13c05c reports UPDATED at this exact revision.
Collection 70a6ed0e9e277fd99344c05bd71f4edf at 01:46:54Z/56Z contains 24 current-build
events on each PC, CONNECTED on both, service/input READY, and no current-build
fault/startup-failed event. Both contain an initial optional source heartbeat
deferral; this is not a physical input acceptance record. A doctor independently
reports healthy CONNECTED and matching UI/resident/installed revisions, verified
launch record and completed update cleanup. No manual reconnect was necessary.

No additional standalone installed-input smoke ran beside the healthy resident
because its singleton conflict was already observed in g405. Hosted CI/push and
race detection were not run. This round changed no API/DB, repository hygiene,
runner, dependency, firewall or signing policy. The only version change is the
workspace bundle generation required for the authorized paired installation.

## Remaining work

Remote inspection still has no open PRs. Dashboard #1 stays open by request;
#4/#5/#6/#7/#15/#16/#20/#50/#59/#70 retain their unresolved acceptance conditions.
No issue comments, unjustified closures, branches/worktrees or remote ref updates
were made. The user's earlier g377 input confirmation remains valid for that
build only; home/study pair identity and signing availability remain unanswered.

Next: #59's native hook callback -> Scope.Do -> desktopBinding.active UOI_IO
path, preserving its required Windows-thread/desktop affinity. Do not move native
callback work to a detached goroutine or infer physical held-input cleanup from
adapter tests. #50/#59 closure still needs the remaining call-graph/native and
physical evidence. No new implementation unit was started after ten minutes.
