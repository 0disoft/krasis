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
