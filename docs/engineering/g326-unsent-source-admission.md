# Unsent source admission expiry

Paired g325 snapshots `38d837caa129fae9af1c2d14ab94cc01` and
`122be2f5df528d9991f1a15cb264d234` preserve distinct failures on A. Source
heartbeat failures were observed at 17:22:34Z (response timeout) and 17:23:40Z
(authority stage). After a safe stop and relaunch, trust freshness expired at
17:28:57Z, followed by unsuccessful source retirement. Both protected services
remained READY. The latter failure must not be attributed to heartbeat alone.

A deterministic regression reproduces a separate admission race: the caller
deadline expires between the entry context check and validation of its local
single-use proof. Previously that returned ErrAuthority, closed the authenticated
channel and cleared its active epoch despite sending no request.

Proof validation now preserves context cancellation/deadline errors, while bad
owner/operation proofs and actual authority failures retain ErrAuthority. The
client preserves an owner only for an unsent context failure. Heartbeat polling
classifies that case as unsent maintenance using the unchanged sequence. Sent
frames still need exact replies and retain the existing timeout, drain and
retirement policy. No input is replayed, deadline extended or trust check skipped.

This fixes the reproduced race. It does not establish why the observed g325
response was late or why the trust reader had no fresh completion. Keep the
original snapshots and distinguish restored connection from long-term stability.
No API, peer wire, database, dependency, ACL or runner change.
