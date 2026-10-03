# g138 A protected input installation

Subsequent B physical PIN cancellation results and received diagnostic checks
are recorded separately in the [B recovery result](2026-10-03-g138-b-pin-recovery.md).

## Scope

The operator explicitly requested installation on A (HP15). This records the
local installation and selected backend; it does not claim remote PIN control.
Product generation is 138, revision `c3566d86d2066e8645fe567354966ce4bb356aa7`.
The four executable hashes matched the independent values in the
[g138 recovery request](../requests/2026-10-03-g138-pin-recovery.md).

## Attended installation

- Before installation, doctor reported g138 UI/agent agreement, CONNECTED to B,
  no installed agent/service, and PORTABLE input.
- The authenticated disconnect completed with code 0 and LOCAL was confirmed.
  The same-session UI was matched to the verified g138 executable and stopped
  before the agent's authenticated safe-stop, avoiding UI automatic recovery.
  Same-session UI and agent absence was confirmed before elevation.
- A hidden PowerShell wrapper requested standard Windows elevation. It checked
  the installer hash, administrator membership, and process absence again.
  It invoked the embedded-payload installer once; no reinstall was attempted.
- Wrapper and installer exit codes were 0. stdout reported `code:0`,
  `state:installed`, `diagnosticSaved:true`, `result:0`, `stage:complete`,
  `recovery:not_needed`. stderr was empty.
- The final receipt time was `2026-10-03T11:48:22.2681333Z`. A new sanitized
  support export contained that same installation time and result.
- `KrasisInput` was Running. The installed agent hash matched g138. Doctor
  verified the installation and matched the service's fixed execution path.

## Restart and current evidence

The g138 UI was reopened from the ordinary host process. Doctor reported
`agentLocation:PROTECTED`, `protectedInput:CONFIGURED`, installed agent VERIFIED,
service RUNNING/MATCHED, and UI/agent/install build agreement. A single
authenticated connect request for B completed with `connect_started`.

The initial subsequent status was CONNECTING to B with `inputBackend:PROTECTED`.
Peer management inspection observed B ONLINE at the same g138 revision. The
operator was asked to reconnect from B after the deliberate safe disconnect.
The final authenticated status subsequently confirmed CONNECTED to B with
`inputBackend:PROTECTED`. The operator also confirmed the connection and
reported slight stuttering. That latency remains unresolved; the report is not
confirmation of all keyboard/mouse or PIN checks. Input worker lease readiness,
physical bidirectional input acceptance and PIN responses remain unverified in
this installation record. An ONLINE management channel alone is not evidence
of a connected input session.

A single CLI collection from A saved both sanitized reports with result
`saved`/`RECEIVED`, created `2026-10-03T11:51:53Z`. The exported events filtered
to this build after installation contained no lease-ready or long-bucket timing
entries; that absence does not measure latency or prove no failures occurred.

No Computer Use or PIN test was invoked. No credential, window text, clipboard
or input content was collected. The installer owned service creation/start;
there was no manual service start, transaction deletion, certificate registration
or Windows security-policy workaround. Raw installation outputs remain local
and ignored by Git. Product source and package files were unchanged.
