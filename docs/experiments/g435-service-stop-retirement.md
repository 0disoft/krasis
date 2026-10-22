# g435: retain an already-stopping service until native process exit

Priority: close the installer stop race before investigating further heartbeat
latency. B's g434 directory_retire_move failed with Win32 5 and rolled back;
manual recovery succeeded. The failed move alone does not identify its owner.

The installer previously retained a process only after observing SERVICE_RUNNING.
If its first observation was SERVICE_STOP_PENDING, it could subsequently accept
a clean receipt and SERVICE_STOPPED without joining the process that still owned
installation/image pins. Retain that owner through the same bounded exit wait.
Use the protected service receipt's PID in this transitional state: Microsoft
does not guarantee QueryServiceStatusEx's PID in STOP_PENDING. Independently
validate the live owner's token, fixed image, ACL and digest. An initial native
handle fences PID reuse during verification. A signaled process or nonexistent
PID needs no live token inspection; other lookup failures remain failures.

Regression coverage uses a real process handle that remains alive after fake SCM
STOPPED, for both RUNNING and already-STOP_PENDING entry. Neither may admit
replacement. Also reject failed identity verification and accept the exact
kernel-confirmed exit of an owned sacrificial child. Production SCM is not
modified by these tests. The existing 10-second stop budget and clean-release
receipt remain required; no forced termination or widened sharing/ACLs.

This fixes a code-proven race, not the proven cause of B's earlier failure.
The initially-STOPPED path is unchanged; binding a historical receipt to a live
owner there needs separate consideration of stale/recycled PIDs. Next: identify
the owner if B's failed move recurs, then correlate any renewed ge1s heartbeat
response with g434's worker timings. Do not infer either runtime issue is solved.

No public API, DB, dependency, runner, repository hygiene or product/architecture
source-of-truth document change. Local main only; no remote push/ref update.

Reference: https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-queryservicestatusex
