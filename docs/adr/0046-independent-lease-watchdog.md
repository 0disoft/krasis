# ADR 0046: Independent lease and write deadlines

Status: Accepted

Publish immutable monotonic lease progress outside the input mutex. A separate
worker reads it without acquiring the semantic write gate or input mutex. Active
leases use PeerTimeout; pending handshakes have a five-second budget. Expiry stops
source forwarding, cancels the session and closes the connection to wake blocked
I/O. Final release remains serialized after in-flight injection completes.

TLS frame writes have a two-second ceiling including write-gate admission. A
failed/partially written stream is closed, not reused. Shorter caller deadlines
still apply. Watchdog tests hold the semantic gate without external cancellation
and check both the fake pressed ledger and runtime ledger are released.

This does not yet bound a stuck native input call or prove native owner cleanup;
the subsequent recovery and capture-lifecycle changes address those boundaries.
