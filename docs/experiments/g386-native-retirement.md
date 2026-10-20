# g386: retain native worker ownership and fence failed handle values (#50)

The prior Close discarded the exact child process handle and protected image
pin after a failed wait. It withheld done but marked the worker closed and
could not observe a later exit. Rejected process creation had a second copy of
this ownership loss in its local cleanup. Both paths now converge on the owning
ConsoleWorker: rejected creation transfers its process handle even on error.

Close closes the kill-on-close job once and waits for the exact process for at
most its existing five-second budget (one second on the login path). Only
WAIT_OBJECT_0 permits process/image cleanup and done. WAIT_TIMEOUT retains the
process and image in one process-wide owner; a single reaper polls without
waiting. Pending retirement rejects replacement console launches and is also
checked during launch authorization. A retiring worker cannot be adopted by a
prepared child or bootstrap. Completed handle retirement cannot repair a lost
release receipt or erase the independently latched service recovery failure.

Unlike the rejected g381 implementation, failed handle operations are never
retried. Wait errors, unexpected wait states and failed handle closes poison
the owner. A later numeric handle value might identify a different resource;
the reaper and exit diagnostics do not touch it again. Its object/image remain
retained, new launches remain fenced and service process exit is required for
OS-owned cleanup. Once only poisoned records remain, the reaper stops polling.
These records cannot accumulate through repeated replacement launches because
launch admission remains closed. No native input is detached or replayed.

After authenticated bootstrap, supervisor/watcher polling checks only the exact
pinned process and console-session presence. These loops grant no input or
recovery authority. Full fresh WTS/desktop/lease checks still run before actual
commands and release recovery. Stop requests and final recovery-report reads
therefore no longer wait behind a separate synchronous WTS token RPC.

## Local validation

Inputworker and residentagent suites and go vet passed. Tests cover delayed
exit, retained image/process ownership, replacement launch denial, exact-once
job/process closure, real hidden child termination through a kill-on-close job,
eventual exact kernel exit observation and concurrent/zero Close. Failure cases
verify no repeated wait/close or fabricated done after job-close, wait,
unexpected-wait and process-close errors. Propagation-only tests now use a
completed synthetic failure rather than a permanently invalid native handle.

A supervision test holds full authorization blocked while the pinned-presence
monitor sends stop and consumes the exact report. A remaining pressed token
still yields ErrRecovery; successful handle cleanup cannot fabricate release.

The race detector remains unavailable (CGO disabled; no C compiler on PATH).
No hosted CI, forced physical lock/suspend/network fault trial, signed-release
validation or new physical input acceptance is inferred. Paired application
must verify protected source readiness, including the g381/g384 startup failure
pattern, before this implementation is accepted.

## Remaining scope

This completes another #50 native-lifetime boundary, not the full cross-backend
acceptance audit. Portable in-process injection and crashguard admission must
still be checked for operations ignoring cancellation. #59 mandatory callback
work and the explicit hardware/accessibility/signing issue gates remain. Do not
close an issue solely because these local tests passed.

No public API, DB, dependency, runner, firewall, signing or trust-policy change.
Only implementation and this experiment record change. Local main only; no
push, new branch/worktree or GitHub comment.

## Paired application

Both PCs applied 6f02f87e6f5f70c2061692e58456d125a7944651. UI's 85 tests,
native builds, installer provenance and eight-file archive validation passed.
Update verification accepted 27,981,008 bytes; B receipt
a96958e82fc24af302dbe72659104462 reports UPDATED. A installer returned 0 and
its installed agent hash matches. No Disconnect was sent during replacement.

Doctor reports healthy/CONNECTED with matched running/installed images and
completed stage cleanup. Paired collection eebd7565df248f80fb93d400b5038d4f at
21:08:39Z/41Z reports READY input health and one protected source ready event
on each PC. The current-build 22 A / 49 B events contain no fault_ or
session_failed. This verifies short-window source startup after the g381/g384
failures; it does not establish their cause or replace physical fault trials.

The second task started before this run's ten-minute cutoff addresses a
separate #50 finding: crashguard invoke starts a goroutine per caller even when
an earlier protocol call is stuck. Admission and permanent retirement will
bound that resource path. Portable in-process SendInput remains a separate
native boundary; #50 remains open pending its full cross-backend audit.
