# g138 B protected input and remote PIN recovery

## Evidence boundary

The operator supplied B's completed installation report and physical A-to-B
input results. A independently inspected the received sanitized B support file
created `2026-10-03T11:55:32Z`; this agent did not repeat the physical tests.
Product generation is 138, revision `c3566d86d2066e8645fe567354966ce4bb356aa7`.
A's own installation is recorded in the
[A installation result](2026-10-03-g138-a-protected-install.md).

## Reported B installation and physical checks

- Four executable hashes and the package checksums matched the independent
  [request](../requests/2026-10-03-g138-pin-recovery.md). Files were NotSigned.
- Installation was invoked once: exit 0, `code:0`, `state:installed`,
  `diagnosticSaved:true`, `result:0`, `stage:complete`, `recovery:not_needed`.
  stderr was empty. `win32Error:0` with `win32Known:false` is an unreported
  native error value, not evidence of installation failure.
- The receipt time was `2026-10-03T11:39:53.2836827Z`. The received support
  contains that same receipt time, result and completion stage.
- Service Running, installed agent hash and input manifest agreement were
  reported. Doctor returned 0/healthy, PROTECTED/CONFIGURED, verified agent and
  matched paths. B status was CONNECTED to A with inputBackend PROTECTED.
- Ordinary movement, clicks, keyboard, B-to-A return and connection retention
  passed according to the operator.
- A's keyboard Esc dismissed B's Chrome Windows PIN dialog. After reopening,
  A's mouse successfully clicked Cancel. Neither trial used B's touchpad or
  built-in keyboard for manual dismissal. PIN value entry was not tested.
- B reported the post-test diagnostic transfer as SENT. A inspected the
  received file rather than treating QUEUED as successful delivery.

No Computer Use, credential capture, security-policy changes, repeated
installation, transaction deletion or manual service start were reported.
Service and installed files were retained. These observations do not establish
all secure-desktop behavior, B-to-A PIN control or signed external readiness.

## Remaining latency

The operator reports slight stuttering. In the received export, events filtered
to this exact product revision show 22 lease-ready records, no recorded connect
or heartbeat failures, and one incomplete source log. Missing records cannot
be treated as proof that no failure occurred.

Actual-input timing includes two `client_response_lt1s` records, meaning a
100ms-to-under-1s response maximum in each reporting window. These are window
maxima, not counts of slow input events. The response includes worker, delivery
and scheduling costs; it does not identify the cause of visible pointer delay.
Several client/worker stages also have lt100ms records. Maxima across stages
must not be summed. Heartbeat timing must remain separate from actual input.

PIN cancellation control is physically reconfirmed on this pair. Latency
remains unresolved and is the next independent investigation.
