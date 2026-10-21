# g427: retain source reporting ownership across query retirement

The g426 live failure at 12:12:27Z records a missing source report and
capture/worker-report-eof, followed by service failure 1066/specific 2. Source
inspection found that a source worker's service-proof query Channel and final
worker reporting used the same physical control pipe with one close owner.
A mandatory query timeout fences authority; when the retained native query
later returns, Channel closes its resources. That could close the report pipe
before runSourceCapture finishes native cleanup and finishWorkerReport runs.

A new regression extracts the original direct shared-pipe construction into
a factory and reproduces the ordering: a blocked native query expires, its
caller returns and authority is fenced, then the query completes before the
final report. The baseline fails with final report transport retired before
report delivery (0.044s). This is a controlled reproduction of a source defect,
not proof that every g426 worker-report-eof has that cause.

The source service Channel and bootstrap reporter now retain exactly two
references to the same original control pipe. Retiring the query/proof releases
only its reference; bootstrap releases reporting ownership after final report
delivery and the control reader join. The physical pipe closes once both
references retire. A native query that finishes after reporting still retains
its exact pipe/proof until actual completion. There is no dynamic reference
admission, extra native-query capacity, cached authority or new input owner.

Regression coverage checks both completion orders, no early proof/pipe close,
one final physical close, fresh work denied after fencing, preservation of
source-service-query-deadline in the real final wire report and preservation
of ErrRecovery when the query/cleanup remains uncertain. Existing timeout,
provenance and missing-report tests also pass in the focused run (0.061s).
Full inputworker/productpanel suites pass in 24.195s/0.140s. Rollout
observations follow below.

This change fixes a demonstrated loss of final failure evidence. It does not
claim to remove the native authority query delay or prove physical stability.
Genuinely missing reports, failed retirement and unjoined cleanup still fence
the service. No public API/wire schema, DB, dependency, runner, firewall,
publisher-signing or repository-hygiene change. No push or issue comment.

## Paired application

Source revision 9ef9be7aa3e4d32472ef36c930034180b1bfcddf is packaged as g427.
The normal workspace builder passes 85 UI tests, native frontend/executable/
embedded-installer builds, real archive smoke (6.19s) and eight-file verification.
Offline verification accepts the 28,321,418-byte bundle.

B update 60fd5d94db1088fb1d65671d9d6f9ca2 reaches UPDATED at this revision.
The initial 40-second observation times out while the same transfer is still
SENDING; the next wait resumes that exact job, with no duplicate send.
B's installation receipt is result 0 at 12:26:29.6719675Z, service READY at
12:26:32Z. A's initial attempt after user-agent exit still returns result 4,
replace/directory_retire_move/Win32 5, restored, at 12:27:26.7794732Z. The UI
is restored to g426 rather than claiming g427 installation success.

A read-only Windows Restart Manager inspection of the three fixed installed
files lists only the current user agent and KrasisInput service. It is not an
exhaustive handle inventory or proof of which reader blocked the earlier
directory rename; no Restart Manager shutdown/restart action is used.
The next trial also retains and waits for the path-verified UI process handle
and disposes every observed old product process handle before installation.
It returns exit 0/result 0 at 12:30:19.0688295Z, complete, recovery not_needed.
This supports strengthening manual process-retirement procedure; it does not
prove that lack of handle disposal caused every historical access denial.

A app repair/relaunch registers and opens g427. Doctor verifies matching UI,
resident and installed g427 revisions, service RUNNING and protected input.
Collection 587a35337fa2beeaa7a8149c69fa982b at 12:30:34Z confirms both result-0
installation receipts, service READY and inputHealth READY on both PCs.
A's log records CONNECTED at 12:30:40.6813546Z and protected capture READY at
12:30:40.849919Z; live peer inspection reports B ONLINE at the same revision.
Short post-connection observations follow separately and do not replace
physical round trips, user stutter feedback or a supported-device soak.

Four bounded local samples at 12:32:03Z, 12:32:34Z, 12:33:05Z and 12:33:35Z
all report CONNECTED/PROTECTED input and RUNNING service with exit codes 0 and
the same service PID. Thus the local protected capture/connection remains
available at the final sample, about 175 seconds after connection readiness,
past the roughly 112-second g426 failure interval. This is a short, unattended
observation, not an equivalent physical workload or proof of long-term repair.
Final paired collection 8a48a1f0eceb9363742d0564e908aca7 is RECEIVED and final
doctor reports healthy matched g427 builds, visible UI process RUNNING and
CONNECTED protected input. No new failure appears in the inspected recent A
g427 log tail. These bounded results do not complete any remaining issue.

Next priority: obtain a real g427 source-query failure/report if one occurs,
check whether the preserved operation identifies native delay or uncertain
cleanup, and address that demonstrated cause. Physical round-trip/stutter,
endpoint-change, multi-device acceptance and publisher signing remain pending.
No additional implementation begins after the round's ten-minute cutoff.
