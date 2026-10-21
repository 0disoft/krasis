# Issue 16: verified process exit before manual protected-input repair

The g426 follow-up investigates the previously recorded A install failures:
finish/commit/file_delete result 5 and replace/directory_retire_move result 4,
both Win32 5. Those historical results remain in the g426 keyboard-review
record; the later success does not rewrite them.

Source inspection shows that protected installation/image handles deliberately
deny deletion and replacement. The native installer waits for the verified
service process to exit. A manual CLI safe-stop response acknowledges a request;
it does not assert that the user agent and its children have exited. The earlier
manual shell proceeded directly from this acknowledgement to installation.
Native UI maintenance instead owns the startup lease and waits for agent
availability. This investigation does not establish a defect in that UI path.

The first follow-up inspection safely stopped before any mutation because it
expected one installed-agent process and found two. Parent inspection shows a
resident and its child, not evidence of duplicate resident ownership. The actual
trial retained process handles for both path-verified installed agent processes,
stopped only the path-verified old UI, requested safe-stop, and checked both
handles with WaitForExit before starting the same verified g426 installer.
Neither process was forcibly terminated. The elevated installer retained its
normal service-stop and recovery logic; no file lock, ACL or security check was
relaxed, and no journal or protected file was manually removed.

Both user-agent process handles reached exit. The unchanged g426 installer then
returned exit 0. The new installation receipt at 12:08:49.8656586Z reports result
0, complete, recovery not_needed. The fixed Program Files directory contains
Input and normal service/login/transaction-lock records; Input.stage,
Input.rollback, Input.retired and pending transaction journals are absent.
The UI was relaunched as g426 and the existing saved peer was connected again.
After its acquisition window the CLI reports CONNECTED with PROTECTED input.

Collection cc03cbdb63d0841da402be999ba50631 at 12:10:29Z/12:10:30Z confirms A's
new result-0 receipt, B's result-0 receipt at 11:59:43.5928985Z, and both input
health states READY. Doctor observes matching g426 UI/resident/installed builds
and running service; it initially sees CONNECTING, while a later status reaches
CONNECTED. These observations do not establish physical input or soak results.

The successful unchanged-image comparison supports incomplete manual process
retirement as a cause of the earlier replacement failures. It does not prove
that every access-denied installation failure has that cause. No installer
retry logic or executable change is justified by this trial. The installation
runbook now distinguishes request acknowledgement, actual process exit and
terminal installer receipt, and documents the already-supported repair action.

Focused native image-pin regressions pass (0.417s): duplicate ownership retains
write/rename exclusion after the original installation closes, and closing an
image proof does not release the installation's pin. git diff --check also
passes. Full suites and a new package build are omitted because executable
sources are unchanged; this check is not a substitute for the missing matrix.

Issue #16 remains OPEN: publisher signing and the broader install/remove/data/
firewall/interruption/rollback acceptance remain unverified. No new build,
version bump, remote push/ref update, issue comment, API/DB, dependency, runner,
repository-hygiene, certificate or firewall change is made in this round.

## Subsequent runtime failure: installation success is not service stability

Before finishing the round, fresh doctor contradicts continued healthy protected
input: A's service is STOPPED, SCM exit 1066/specific code 2, and the replacement
agent uses portable input. The last successful installation receipt remains
result 0. Do not turn that installation success into a stability claim.

Direct agent log inspection records CONNECTED/protected source ready at
12:10:34Z, source reader EOF and failed heartbeat at 12:12:26.894Z, followed by
fault_source_hook_failed and session_failed. LOGIN-RUNTIME.json records service
FAILED at 12:12:27.0171177Z: capture, worker-report-eof, missing source report,
and worker exit RUNNING/unknown when inspected. The source exit fields are
unknown; they do not prove that native cleanup joined. No matching Krasis
Application crash/hang event was found in the inspected 12:11:50Z-12:13:00Z
window; that limited negative result does not establish an external kill or
exclude a worker failure. Diagnostic request 6787707a19566e5a3174bca71c86d148
was polled to its terminal EXPIRED state rather than treated as received.

Code inspection identifies a candidate reporting-lifetime problem, not yet a
reproduced root cause of this runtime event. The source worker's service proof
Channel uses the same control pipe as the final worker report.
channel_authority.go fences and eventually closes that Channel after a mandatory
query expires. bootstrap_windows.go subsequently calls finishWorkerReport on
the same control pipe after source cleanup. A query owner returning before
final report delivery can therefore close its report transport. The source
service must continue fencing a genuinely missing/uncertain cleanup report.

Next implementation step: reproduce expiry/late query completion around the
final source report; if confirmed, separate the report owner's control-pipe
lifetime from the query owner's reference. Keep the exact native-query pipe
alive until actual return, keep authority fencing and process-wide query bounds,
preserve all failed/uncertain cleanup outcomes, and close the physical pipe only
after both report delivery and query ownership retire. A bounded failing report
must never become successful retirement. Add focused regressions before changing
the shared resource lifecycle. No such fix or test is claimed in this round;
the ten-minute new-work cutoff has passed.

Recovery uses the unchanged g426 repair after path verification and actual exit
of all three portable agent/child process handles. It returns exit 0/result 0 at
12:18:07.8007471Z. The UI is restored at g426 and a saved-peer connect is started.
Doctor observes matching UI/resident/installed builds and RUNNING service with
PROTECTED input, initially CONNECTING. Collection e91146012136ff734f00feebf61ae9e8
is RECEIVED. This recovery is not a fix for the source-worker failure and is not
grounds to close #6/#7/#16/#20.
