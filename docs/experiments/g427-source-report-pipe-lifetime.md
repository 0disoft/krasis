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
