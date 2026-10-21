# g422: distinguish source query ownership and failure reasons

g421 paired collection `03665bb12eceeda393f3d6d7df5d33c6` records an A
source-reader-authority failure while B's source retires cleanly. The source
watcher projects query deadlines, capacity exhaustion and channel shutdown to
the same ErrAuthority as a real native rejection. That projection erases the
evidence needed to select a repair. A subsequently falls back to PORTABLE
after its protected service reaches recovery-required (1066).

Retain closed, numeric report operation codes for source query deadline,
cancellation, capacity and closed/fenced channel, and identify service versus
agent in source read authorization. The watcher retains only these fixed codes
and its original ErrAuthority sentinel classification; arbitrary text/identifiers
are not serialized. Concrete native authority/cleanup failures keep precedence.
Append report operation bytes without changing the wire layout or old codes.
Ordinary input/heartbeat APIs, query slots, deadlines, fencing, handle lifetime,
injection and native retirement semantics remain unchanged.

Deterministic tests exercise a real timed-out query against each pinned owner,
then watcher projection, final classification and the report wire. They also
distinguish capacity from closed state and preserve concrete native rejection.
The first wider suite fails only the three portable-pair recovery fixtures
because A's real PORTABLE session holds their fixed ownership object. Existing
safe-stop succeeds before rerunning unchanged inputs without that owner.
No production fence or test assertion is weakened.

This is a diagnostic repair, not a claim that reconnect is fixed. Paired rollout
must produce a specific query failure before the next behavioral change.
No issue closure, push, comment, remote-ref, API/DB, dependency, runner,
firewall, signing configuration or source-of-truth specification change.
