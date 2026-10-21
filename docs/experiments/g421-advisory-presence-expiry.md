# g421: keep unsent advisory presence expiry from fencing capture

The g420 reconnect trial identifies repeated A source-worker failures; the
initial generic source-command-loop record does not establish its exact cause.
Inspection found a reproducible remaining failure in the advisory boundary
sender: its full proof uses optional maintenance ownership, but its subsequent
pinned-process presence check uses mandatory command presence ownership.
A one-second delay in that check fences the entire capture channel even though
no observation bytes were submitted, and prevents independent heartbeats.

The new deterministic regression fails on g420: an expired advisory presence
query retires capture and rejects a fresh heartbeat. Use the existing optional
maintenance owner for this advisory-only presence query, discard only its
unsent expiry while the capture owner remains live, and require a fresh check
for subsequent work. Pending queries retain the same bounded slots/handles;
Close still requires their retirement. The command presence lane, full input
authority, native checks, wire order, attempted/partial writes, and native
retirement rules are unchanged. A real presence denial is still fatal.

Focused boundary, source writer, source retirement, unsent proof, authority
timeout and native pair tests pass. The new regression checks no transmitted
bytes/sequence consumption, heartbeat independence during a retained optional
query, and rejection by a fresh proof after the late query joins.
This fixes a demonstrated route to reconnect, not every source-worker failure
seen in the paired history. Paired application and follow-up evidence follow.

No push, issue comment, remote ref, API/DB, runner, dependency, firewall,
signing configuration or source-of-truth specification change.

## Verification and paired application

The full inputworker suite passes (24.913s), including portable pair fixtures,
and logindiag passes. The normal workspace builder passes 85 UI tests, native
frontend and Windows executable/embedded installer builds, package archive
smoke and eight-file verification. Offline verification accepts generation 421
at `52e8da0616ad724ad9b02f709d5b1bd1f6694f2a`, 28,296,957 bytes. Hosted CI,
physical held-input/Winlogon/fault matrices and publisher signing were not run.

B update job `923725496fa9b9cfe55dcc1f05f582b2` reaches UPDATED at that
revision; installation receipt 10:30:01.832244Z has result 0. A's old g420
service had again reached SERVICE_RECOVERY_REQUIRED (1066). Existing safe-stop
and verified older-UI shutdown complete before its g421 elevated repair.
Installation receipt 10:30:37.0895021Z has result 0/stage complete, and the
service is RUNNING. Collection `af6cd754f6383f2012f9cbe081f67edb` confirms
both installation receipts and READY health on A and B. No Security dialog
input or forced process termination was used.

A's g421 UI/agent/CLI/installed agent match. Peer inspection confirms B ONLINE
at the same g421 revision. A's existing connect command starts the saved B
profile; at 10:33Z status is CONNECTED with PROTECTED input. During A's
installation B logged acquisition timeouts; those replacement-window events
are not a post-connection stability result. Physical round-trip feedback and
the final paired history remain pending at this point.

## Reproduced remaining failure and next step

Collection `03665bb12eceeda393f3d6d7df5d33c6` records another A failure at
10:33:33Z, after the initial connection. A reports source-reader-authority,
joined capture/sender/interrupt, delivery_failed retirement and a failed final
report without a native code. Agent heartbeat response exceeds one second and
its reader reaches EOF. B's independent source retires cleanly and its session
enters RECONNECTING. This is direct evidence that g421 does not resolve the
entire reported reconnect problem; do not close its tracking criteria.

After the existing recovery/reconnect flow, A and B reconnect at 10:33:46Z /
10:33:45Z. Collection `a34dbb12388b9aafd3f7236e396de81f` at 10:35Z shows
both READY and no second source exit since installation. No g421 handoff
profile is present, so this is idle connection evidence, not physical
round-trip acceptance. User feedback is still pending.

Next distinguish read-side authorization from the independent presence
watcher's error, and preserve query stage/reason (service versus agent,
capacity/deadline versus native rejection) before generic source-reader-authority
projection. Inspect source read/presence query ownership and cancellation
races without suppressing mandatory failures or widening input authority.
No new implementation was started after this run's ten-minute cutoff.

The local runtime trace copied during inspection has timestamp 10:22:57Z,
from g420; it is not evidence of the g421 worker failure. A fresh worker-side
trace or stage-specific report is needed before attributing its native delay.
