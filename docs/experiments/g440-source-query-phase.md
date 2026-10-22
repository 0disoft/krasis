# g440: distinguish source authority-query deadline phases

g439 A failed with source-service-query-deadline and a service_presence lifetime
maximum of at least one second. The service presence callback in
identity_windows.go checks GetNamedPipeServerProcessId against its authenticated
server PID; it does not repeat the full SCM binding check. Existing evidence
therefore does not establish that SCM itself stalled.

Retain a fixed phase when a source query's deadline is observed: admission,
waiting for a previous query, scheduled, checking, publishing, retiring, or
completed. A small atomic phase belongs to the retained query. Snapshot it
before cancellation; keep the existing cancellation, fencing, capacity, and
native-handle retirement behavior. An expired waiter must not retire another
caller's query. No proof is cached and no failed request is retried.

Checking covers entry into the authority callback through its return, including
possible scheduling delay. It does not prove an OS syscall spent the whole
interval blocked. Completed means the query finished before the caller observed
its expired context; it is not permission to accept an expired result. These
are observations at deadline handling, not per-substep elapsed measurements.

Append private report codes after all existing values, preserving old numeric
operations and frame layout. Preserve the phase through the existing sanitized
watcher projection. Native denials and uncertain cleanup retain precedence.
Only fixed owner/phase categories survive; no input, identity, path, or error
text is exported. Public API, DB, dependencies, runner, repository hygiene, and
product/architecture specifications are unchanged. Local main; no push.

Validation includes a real previous-query waiter deadline without ownership
transfer, stalled service/agent callbacks, final-report/native-query ordering,
phase report roundtrip, closed export labels, and byte-sized operation limits.
The portable pair fixed-owner fixture remains excluded to avoid live-session
conflict. Packaging, paired application, and observations are recorded separately.
This identifies a diagnostic blind spot; it is not a latency-resolution claim.

Passed: inputworker 22.629s, logindiag/productlog/residentagent (cached), and
git diff --check. Race detector, physical-input acceptance, long soak, and hosted
CI were not run in this bounded change.

Fresh g439 collection 034ef4a01bfc4fe54338ed44bebfc3f0 also caught distinct A
reply-gate failures at 14:48:06Z and 14:49:08Z: the observed holder was boundary
in write and prepare respectively. Both native/sender owners joined, with the
service still READY. These samples identify advisory boundary traffic as an
observed gate holder, not proof that it occupied the full waiting interval or
that the reader/pipe/encoding caused the delay. Keep this separate from the
earlier service-query deadline; a shared scheduling cause is still unproven.

Paired rollout succeeded: B UPDATED; A installer exit zero; app repair
recovery_ready. Collection 73c01e62e0096d03ea0f7c481b30632a confirms both input
health/service states READY and final doctor healthy/CONNECTED with matching
g440 images. No source exit or source-command failure is recorded in either new
service lifetime through 14:57:04Z/14:57:07Z. No query deadline was captured;
this short observation does not establish intermittent stability. Latest
heartbeat aggregate response buckets are A lt10ms and B lt1ms, not a controlled
before/after comparison. Rollout checks: 91 frontend tests, native build,
6.75-second package smoke, eight-file bundle, and offline verification passed.

Next prioritize a regression through the actual readSourceCommandAuthorized
failure path. Its unprojected query deadline retains context.DeadlineExceeded,
while the watcher projection retains only ErrAuthority. Check whether a query
that has already retired can therefore be classified as uncertain native
cleanup in only the read path. This is a static candidate, not a reproduced
runtime cause. Any fix must retain separate pending-query/handle and native
cleanup proof; do not merely suppress ErrRecovery or relax mandatory queries.
Results: results/2026-10-22-g440-source-query-phase.json. No further
implementation was started after the ten-minute cutoff.
