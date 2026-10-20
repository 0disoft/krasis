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

## Paired application

Both PCs applied g397, revision 93abb659ed5d6cc6fd97b50bf974f49c5b625423.
A installer exited 0 and installed agent SHA-256 matched. UI 85 tests, native
builds, installer provenance and eight-file package checks pass; the verified
archive is 28,118,141 bytes. B receipt 85393a894f2bec6aaf1e55d4fefda4f2 reports
UPDATED. No native injection code changed, so the g396 ordinary installed
cursor smoke was not repeated, and no Winlogon or physical held-input claim
is made.

The first mixed-build connection failed at 23:38:41Z on B's still-g396 source;
A g397 observed cancellation/remote EOF and its service retained joined source
owners with failed retirement delivery. Collection 650ab8c26d20bca9b1a7fa0d5996b474
preserves the paired logs. B's old service history was replaced by its update,
so this does not yet exercise the new missing-report startup-stage evidence.
After A's old process was confirmed gone, its same installed g397 agent was
started and explicitly connected. Collection b9a04346e9357ee24671996f9163f4aa
records both protected captures ready and healthy A doctor.

A further single same-build maintenance restart waited for the exact previous
agent processes to exit; no Disconnect or B reinstall was used. Collection
b6f6881e1e7d6c36c1bf76350d183ad1 records clean joined source retirement on both
PCs, B RECONNECTING, and the subsequent A doctor confirms CONNECTED. This trial
does not reproduce the startup EOF or prove it fixed. All ten issues remain
open. Source changes and this application evidence are local main commits only.
