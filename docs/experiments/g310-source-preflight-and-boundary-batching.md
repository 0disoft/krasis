# Source capture IPC admission and bounded pointer batches

g309 paired collection `bc30ffe9d7a48570d2100a682212cbe9` has 12 matched
successful handoffs. Every B entry uses a prepared child. A's first activation
still takes 89.385ms. Attempt 9 waits 223.616ms for its grant; B's matching
request-read to grant-write span is 28.749ms. A's grant reader spends 222.556ms
waiting for bytes, not dispatching a previous frame. These spans do not identify
TCP transport versus OS/goroutine scheduling and are not cursor rendering time.

The same collection retains an earlier source failure at 16:12 UTC, before the
16:24 handoff trial. Its reader timed out, native capture/sender joined, and the
peer session subsequently reconnected. Coarse heartbeat authority timing includes
one >=1s observation; aggregate buckets do not prove which query caused the exit.

## Heartbeat preflight without the ordered IPC gate

Previously an idle heartbeat held sourceClient.mu through the synchronous SCM
authority query. A capture activation had to wait for that unrelated query.
Authority.mu also serialized independent read-only handle checks. It now permits
parallel readers; Close exclusively waits for all checks before releasing pins.
The actual process, console/logon, service state and pipe-owner queries are kept.

Heartbeat admission checks demand/quiet age, releases the IPC gate, performs its
full authentication, then only tries the gate immediately. It rechecks command
demand, reply age, cancellation and closed authority. A one-use proof can only
authorize that heartbeat, never capture or cursor commands. A command that ran
during preflight makes maintenance unnecessary. An expired *unsent* preflight
does not consume a sequence, fabricate an ACK, renew liveness or close the owner.
Failed authentication remains fatal. Already-written requests retain their exact
reply, one-second budget and cleanup policy; the worker's 2s idle limit is unchanged.
An expired unsent preflight emits one fixed `installed_source_heartbeat_preflight_expired`
code per capture owner; it cannot spam input logs or be mistaken for a sent timeout.

Capture commands also recheck their context after synchronous authentication,
before publishing an epoch or writing. Authentication is not asynchronously
abandoned, and shutdown still joins native owners and retained handles.

CaptureProfile appends four process-local spans: IPC lock, authority, write and
reply wait. Existing indices stay unchanged. CaptureSource now forwards the
current attempt context to those spans; no command contents or identities are
added to diagnostics. Unmeasured worker/native subspans remain unmeasured.

## Already-queued boundary notifications

Only advisory local pointer notifications are drained in batches of at most 16
64-byte packets. No collection timer is added. Pending authenticated commands
stop draining. All edge, retreat, direction, binding and sequence samples are
preserved, with no coalescing and no change to the remote input sender. One fresh
authority check covers one bounded (<=1KiB) pipe write instead of up to 16 checks
and writes. A single notification is still sent immediately. Invalid packets
and authentication failures send nothing; partial delivery remains fatal.

This reduces synchronous Windows queries and context switches competing with
capture/peer dispatch, but does not establish the cause of the 223.616ms network
grant wait. Already-written frames are never reprioritized or retried. No socket
security/power policy, lease timeouts, SCM ACL, service startup or OS setting changes.

Tests verify overlapping read-only proof checks plus joined exclusive close,
activation while heartbeat authentication is blocked, unsent deadline expiry,
one-use heartbeat-only proof, all packet samples/sequences, one check per batch,
invalid/unauthorized batch rejection and bounded draining that yields to commands.
