# g358: Preserve source lease failure details

## Incident evidence

The attended g356 movement trial was acceptable and had no retained fault in its
short window. Subsequent collection `36c243f011e8f7049614ed6e851bf110` contains the
first later A source-hook failure at 2026-10-19 14:22:36.9782177Z. The protected
service stayed READY, while its source history retained a failed report with
clean native retirement and joined capture/sender owners.

Source lease lifetime is `maxLeaseDuration - one second`, and maxLeaseDuration
is one hour. The roughly ten-minute failure time does not match that expiry.
The failure's 64-window GC trail instead retains two >1s total-GC pause counts,
each accompanied by a >1s stopping-GC count. Their timer lateness is 1147327us
and 3822417us. These histograms give bounds, not exact pause durations. Overlap
supports a process-pause hypothesis but does not prove which thread or OS/runtime
condition caused it, nor that GC alone caused the subsequent authority failure.

Go distinguishes [time spent stopping threads from total GC pause time](https://go.dev/src/runtime/metrics/doc.go).
The latter includes the former; do not add them. The retained runtime-stall.trace
was saved later, at local 22:49, and its parsed profiles cover another short
window. It cannot explain the first 14:22Z failure. Do not attribute an unrelated
trace's ordinary syscall waits to that failure.

## Lost evidence boundary and change

The source endpoint already receives the authenticated worker report and the
joined result from runPreparedAgentLease. Its observer previously received only
the report. recordSourceExit retained command/reader/join/report categories but
discarded the returned typed operation and native error. A safely retired source
could therefore remain report:failed with no inspectable original operation,
without promoting a service-wide failure.

The observer now passes that same result to the existing service-owned recorder.
Each source exit may have a failure object containing only the allowlisted
operation, numeric Win32 error and explicit known flag. An unknown result stays
empty operation / zero / known:false. Nil results omit failure. Report status,
native-retirement evidence, service-wide LastFailure and recovery decisions are
unchanged. A service-side final lease error can coexist with a clean child
report, so those fields remain independent.

This metadata is added only after the service consumes and joins the child
result. Worker report encoding rejects service-owned failure metadata; the
authenticated wire layout and operation codes do not change. No error text,
input, PIN, process identity, path or deadline extension is added. Publication
retains the existing immutable, bounded 16-exit history and never waits for
persistence. The source channel's later Close error remains handled separately
by service recovery and is not substituted for the worker lease result.

## Validation and current boundary

Inputworker, logindiag, productlog and localapi tests pass, and
`go vet -unsafeptr=false` passes for those packages. The first test run exposed
one old test call missing the new result parameter; it was updated and the
related suite rerun successfully. New tests cross the actual report encode/read,
service recorder and support sanitizer boundaries, preserving original operation
and native error, unknown-error flags, clean retirement, live service state and
unmodified incoming report. Service-only metadata is rejected on the child wire;
private operations and contradictory native flags are rejected. Existing
bounded-history, nonblocking-publication and persistence-failure tests remain.

Generation 358 is reserved locally. It is not built or applied: both PCs retain
g357 while its attended movement comparison is pending. Collection
`99e6d4026aecc5570e046ed4163c7018` was received and current A doctor was
healthy/CONNECTED/PROTECTED/CONFIGURED before this local change. This patch
repairs evidence loss, not the underlying stall. The next failure's preserved
operation will narrow the remaining authority/worker-retirement investigation.
Existing g356 evidence cannot recover information already discarded.

Bugfix, desktop-app and error-message-integrity review were applied. No API,
DB, runner or repository-hygiene change, push, public release, CI, installation,
service restart, deadline relaxation or new live profiling was performed.
