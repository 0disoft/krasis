# g363: apply locally verified input and source failure fixes

## Baseline and scope

On 2026-10-20 the operator authorized investigation, a new paired trial build,
application, and an attended lock/PIN/login and typing trial. No remote Git
push, public release, reboot, or trust-policy change is authorized.

Before application A still runs g362 (1857565), connected to B, but its
KrasisInput service is STOPPED and doctor reports input_service_stopped with
PORTABLE input. B is ONLINE on g362 and allows the paired development update.
Collection 38c7dafda61bbea5599d5f5d6a18d463 preserves both sides before changes.
The original A runtime receipt and rotated agent log are additionally preserved
under .git/krasis-improvement/g363-preapply, outside tracked sources.

A's service failed at 2026-10-20T08:13:19.1380446Z. Its final source report
records command=failed, reader=stopped, capture/sender/interrupt=joined,
retirement=failed_sent, report=recovery, operation=source-command-loop,
and no known Win32 error. A heartbeat response exceeded one second at
08:13:17.5752369Z; the subsequent stop was rejected at 08:13:18.9616882Z.
This establishes ordering, not the original Go command error. In particular,
the input-channel heartbeat fix is not proof of this source-channel cause.

The trial includes the committed changes since g362: bounded unsent-input
authorization, heartbeat progress during input authorization, preparation
frame validation, pipe-close diagnostics, noninterfering failure observers,
pre-native source activation rejection classification, and command identity
in final reports. Native cleanup and unknown-command recovery fences remain.

## Acceptance and recovery

Use the existing generation-scoped workspace builder and offline update
verification, then send to B and wait for its exact generation/revision.
Apply the same bundle to A through normal single-instance replacement.
Confirm matching UI/agent/installed-agent metadata, running input service,
CONNECTED and PROTECTED/CONFIGURED, and fresh paired diagnostics.

Retain g362 and the saved failure evidence. If g363 fails, collect its first
failure before further replacement and safely stop the affected input session;
do not loop restarts, disable fencing, or silently roll back to the already
failed g362. Recovery must use a verified existing package and preserve trust.
Real lock/PIN/login, rapid returns and ordinary-window typing still require
the operator's observations; startup readiness alone is not stability proof.

No public API, DB, dependency, CI runner, or repository hygiene changes.
