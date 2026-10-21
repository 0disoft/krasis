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
