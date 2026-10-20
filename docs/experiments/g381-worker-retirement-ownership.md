# g381: retain native worker ownership after an unconfirmed join (#50)

ConsoleWorker.Close previously marked the worker closed and released its process
handle and protected-image pin even when the five-second process wait timed out
or failed. It withheld the done signal, but discarded the resources needed to
verify a later exit. Its sync.Once prevented another retirement attempt.

Failed retirement now keeps the exact process handle and image pin. A successful
job close is recorded once; process/image cleanup follows only confirmed native
process exit. A process-wide retirement owner retains failed workers and polls
them without waiting. New console-worker launch admission is fenced while any
retirement remains unconfirmed. One reaper handles all retained workers and
exits when empty. It never injects, replays input, clears a pressed journal or
turns a missing/failed recovery receipt into success. Service recovery fencing
remains independent of a later successful handle cleanup.

The post-bootstrap supervisor and standalone console watcher now use the
existing pinned owner-presence check. These loops only decide when to stop an
already authenticated child; they cannot authorize input. Actual command,
capture, desktop, peer and lease checks remain fresh and unchanged. The service
stop/recovery loop no longer issues a WTS token RPC that could stall behind the
worker's native operation. Bootstrap still performs full authorization.

## Verification

Focused retirement tests pass for wait timeout, wait failure, job-close failure,
process-close failure, launch fencing and eventual resource retirement. A real
Windows test launches a hidden child, assigns a kill-on-close job and retains
an exact native process handle plus image file while exit observation is delayed.
The later real kernel wait completes retirement; no input is injected.

A supervision test holds a full input-authority check indefinitely while the
pinned-presence monitor still consumes the exact completed worker receipt.
The complete inputworker suite passes after the final test updates. The
residentagent suite also passed with the implementation changes.

The first full inputworker run aborted inside Go runtime.preemptM with
DuplicateHandle error 6. A fresh full rerun passed. Existing propagation-only
tests were then changed to avoid leaving impossible native handles permanently
retained by the new reaper; a further complete suite passed. The runtime abort
was not reproduced or attributed to a product input failure. It is not recorded
as a successful test run. No hosted CI or full physical fault matrix was run.

## Remaining work

#50 remains open. In particular, client-side full SCM authorization is still a
synchronous native query before request submission and can exceed its caller's
budget. A safe next slice is an owned read-only authorization-query lifetime:
on timeout, reject submission and permanently fence that proof, retain its
handles until the query returns, bound replacement/query resources and avoid
making Channel.Close wait indefinitely. A late success must never authorize
input or clear uncertain pressed-state/recovery evidence. Do not detach actual
injection or replay it on another backend.

#59 still has mandatory native desktop/cursor work and semantic gates. #70's
physical endpoint-change acceptance and #4/#5/#6/#7/#15/#16/#20 hardware/signing
requirements remain separate. Questions about H-A/H-B availability and a code
signing certificate remain unanswered; no acceptance is inferred.

No public control API, DB, dependency, runner, firewall or signing change. No
push, issue comment, new branch or worktree. Changes remain on local main.

## Application rejected and rolled back

Both PCs installed g381 with successful installer receipts and READY input
health, but the first paired connection failed during source worker readiness.
A's source report joined capture/sender/interrupt after peer EOF; B latched
fault_source_hook_failed without a completed source report. A local agent
restart did not restore B's faulted connection. App-open confirmed B's existing
window but did not clear its process fault latch. The new code is not accepted
as stable and no issue is closed from this application.

The exact relationship to the new retirement code is unproven. Restore the
previous g380 implementation as forward generation g382, retaining this record
and the rejected implementation commit for a later isolated investigation.
The native retirement proposal and client-side SCM work remain outstanding.
