# g441: distinguish rejected read authorization from incomplete cleanup

Reproduced before the fix with
TestCompletedReadAuthorizationDeadlineDoesNotRequireServiceRepair. An encoded
heartbeat passes through readSourceCommandAuthorized, a service presence query
expires, then its native callback and retained handles finish. With capture and
sender already joined, sourceCaptureFinal still adds ErrRecovery. The actual
read path retains ErrAuthority plus context.DeadlineExceeded, while the watcher
projection used by earlier tests retains only ErrAuthority. The two paths
therefore disagreed about a failed but already-retired read-only query.

Recognize only a reader-authority rejection containing a valid fixed source
query operation and an error tree made solely of the authority sentinel and
context cancellation/deadline. The rejected command never reached dispatch.
Preserve the failed command and query details; do not revive the fenced proof,
accept expired input, replay a command, widen a deadline, or suppress native
cleanup errors. Unmarked deadlines and unknown sibling errors remain fenced.

Before publishing the retirement status, independently check both agent and
service channels for retained native queries. Any pending query or missing
channel still requires recovery. Preserve the later channel-close and service
query checks as well. This moves pending-query proof ahead of the agent's final
notification, rather than relying only on the later authenticated service report.

The regression now passes. The existing real query/report lifetime test now
uses the actual encoded command reader, and covers both query-before-report
and report-before-query orderings, pending-handle retention, failed command
provenance, retirement status, and exact-once cleanup. Independent native/sender
failure, unknown operation/cause, bare query error, and missing-channel cases
still require recovery. Shared query operation validation removes duplicated
allowlist logic without changing watcher projection.

Passed: inputworker 22.051s, logindiag (cached), productlog 1.316s,
residentagent 5.952s, and git diff --check. The portable pair fixed-owner fixture
was excluded to avoid the installed session. No race detector, hosted CI,
physical-input acceptance, or long soak in this bounded change.

No public API, frame layout, DB, dependency, runner, repository hygiene, or
product/architecture specification change. Local main; no push. This corrects
a reproduced service-recovery misclassification; it does not explain or resolve
the underlying query/scheduler, boundary-gate, or network latency. Paired rollout
and observations are recorded separately.

Paired rollout: B reached UPDATED; A installer exited zero and app repair
returned recovery_ready. Both installation receipts succeeded. Collection
9f72843f28c8a2db7fe8c3938954be0f confirms both services/input health READY and
final doctor healthy/CONNECTED with matching g441 images. No source exit or new
source-command fault is recorded through the 15:16:46Z/15:16:49Z UTC snapshots.
The new installation has only a short observation window; no live query timeout
was induced or captured, so the regression proof is separate from runtime
acceptance. Package checks passed: 91 frontend tests, native build, 6.48-second
bundle smoke, eight-file inventory, and offline verification.

Before deployment, g440 A was already stopped with Win32 1066 and a
source-command-loop recovery report. That generic loop failure is not proven
to be this reproduced defect. Next preserve its independent cleanup details if
it recurs; keep boundary-gate and underlying scheduling/network delays separate.
Results: results/2026-10-23-g441-completed-read-query-retirement.json.
