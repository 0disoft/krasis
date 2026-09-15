# ADR 0038: Explicit peer disconnect stops product reconnection

- Status: Accepted

## Contract

A user disconnect on an authenticated product connection sends protocol-v1
message kind 4 (`MessageDisconnect`) with an empty payload before closing.
This is connection-scoped, not lease-scoped: it applies in LOCAL, pending and
active states. It does not revoke pairing or grant any input permission.
Nonempty payloads are invalid and do not count as intentional disconnect.

The receiver runs its existing pending-lease abort, source deactivation and
input release cleanup before publishing LOCAL. Its supervisor does not retry
after receiving the signal, even if another worker reports a concurrent error.
Cleanup errors remain failures; receiving a stop is not evidence of successful
cleanup. EOF, malformed frames, crashes and network timeouts retain automatic
reconnection policy. Normal pointer return is not a connection disconnect.

The product connection close owner attempts notification once, only for the
local disconnect cancellation cause. It uses the existing transport write gate
and a 500 ms deadline independent of the canceled session context. Notification
failure is logged with a fixed code and cannot prevent local connection close.
No acknowledgment or guaranteed delivery is claimed. If the connection is
already broken or waiting to reconnect, the remote side may need a manual stop.

## Compatibility

Kind 4 is an additive wire extension, but older receivers reject unknown kinds
and may reconnect. Both PCs must run agent generation 11 or later for this UX.
Existing pairing, identity storage and other v1 frames remain unchanged.
No capability negotiation, persisted stop flag, or background auto-connect is
introduced. A new connection requires the existing explicit connect workflow.

## Evidence

Targeted tests cover empty/invalid stop frames, safe runtime cleanup, real
authenticated transport delivery with mocked OS input, no supervisor retry,
preserved cleanup errors, concurrent close, notification timeout and absence
of notification on non-user cancellation. Physical two-PC confirmation remains
a manual smoke check.
