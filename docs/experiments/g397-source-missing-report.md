# g397: retain source failures without a worker report

The source endpoint previously published an exit only when the worker supplied
a final SourceExit. Admission, request reading, console preparation and early
bootstrap failures therefore left no service-side source evidence. The agent
could see only worker_ready/EOF, as observed on g396.

The authenticated source endpoint now retains failed attempts even when no
worker report exists. Service-only report=missing records command, reader,
capture, sender, interrupt and retirement as unknown. It cannot claim that a
worker never started, that native owners joined, or that cleanup succeeded.
Known startup stages are admission, request, validation, prepare, authentication,
transfer, ready and publish. Existing concrete worker reports retain their own
failure; stages annotate only the absence of such a report.

This uses the existing immutable bounded source history and atomic publication,
without disk I/O or persistence locks on the source endpoint. Only closed stage
codes and an observed numeric native error can enter diagnostics. Raw paths,
input, keys, identities and error strings remain excluded. Missing-report state
is deliberately rejected by the authenticated worker wire encoder, even after
stripping timestamp/failure metadata. It never grants readiness or retirement.

Inputworker, logindiag, productlog, residentagent and productpanel tests pass.
New tests cover every stage, unknown-versus-joined state, native error/cause
preservation, failed recovery ownership, private-error redaction, rejection of
arbitrary stage strings and missing-report rejection under every wire status.
An empty successful result creates no failure history. This fixes lost failure
evidence; it does not by itself resolve the underlying startup EOF or close #50.

This extends private support-diagnostic vocabulary only. No native command,
input permission, public API, DB, dependency, runner, firewall, signing, branch
or worktree changes. No issue comments or push. Paired application and any
observed first startup failure are recorded after deployment.
