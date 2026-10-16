# Completed source command replies

The g326 paired collection `b4d66b2b95a70bcf14b2ce886d13160b` includes
source/peer heartbeat failures and later trust freshness expiry while both
protected services remain READY. A heartbeat fault records approximately 2.97s
of timer lateness and 2.94s of queued residence. These observations do not
identify the original delayed native call.

A deterministic regression reproduces a separate source response failure:
the serial reader authorizes a command, native execution completes, then the
writer repeats full SCM/WTS authorization before delivering its known result.
A delayed or failed second query can therefore hide a completed operation.

Completed source replies now require an exact valid request, channel binding,
operation and request sequence. Only the existing reply format is allowed;
presequenced replies, input frames and stop/retirement are rejected. The writer
still serializes sequence assignment, checks its pinned owner's presence under
the wire gate and uses the existing write deadline. Native command admission,
input actions and boundary observations retain their full authorization checks.
Stop and confirmed retirement keep their separate existing ownership rules.

Tests verify that a second full query cannot discard the completed result,
negative results remain negative, an exited owner cannot receive a reply, and
the reply grants no authority to subsequent input. Malformed and unrelated
frames consume no sequence. No retry, timeout extension, input replay or partial
ACK acceptance was added. This repairs the reproduced response path; actual
two-PC latency and lock/PIN stability still require observation.

No public API, peer wire, database, dependency, ACL or runner change.
