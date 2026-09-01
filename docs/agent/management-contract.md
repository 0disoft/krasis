# Agent management revocation contract

- Status: Active application and local transport contract
- Version: `krasis.agent.control-revoke/v1`
- Decision: `docs/adr/0034-exact-active-lease-management-revocation.md`
- CLI adapter: exact-active control commands remain a follow-up to the product CLI projection

## Ownership

The resident agent is the only owner that may revoke an active peer lease. A UI or CLI authenticates to the local agent, requests an operation, and displays the bounded result. It never writes the trust store, closes the peer socket, releases input directly, or changes suppression itself.

The agent session registers its authenticated peer, runtime, write coordinator, source activation owner, connection interrupter, and trust revoker in one `SessionRegistry`. Registration exists only while that exact peer session is alive.

## Local authorization

The local transport uses the current process bearer credential stored under the protected product control directory and maps the authenticated current-user client to a `ManagementPrincipal`. Immediate lease operations require `LEASE_REVOKE`.

Authentication and authorization happen before request parsing reaches `SessionRegistry.Revoke`. An empty principal is unauthenticated. A principal without the capability is forbidden. The principal subject is never included in the operation result or aggregate metrics.

The Phase 0 `krasis-lab` executable does not expose this service and does not create management principals.

## Active target projection

The local API exposes a redacted active-control projection with:

```json
{
  "schemaVersion": "krasis.agent.active-control/v1",
  "state": "LOCAL|INBOUND_ACTIVE|OUTBOUND_ACTIVE|FAULT",
  "peerAlias": "user-selected alias or empty",
  "targetToken": "opaque process-scoped token or empty",
  "actions": ["REVOKE_LEASE", "DISCONNECT_PEER", "REVOKE_TRUST"]
}
```

`targetToken` resolves inside the agent to the authenticated certificate SHA-256 identity, certificate role, session ID, lease ID, and epoch observed in one atomic projection. It is not a credential and grants no authority without local authentication and `LEASE_REVOKE`, but it is still omitted from logs, diagnostics, shell history, and UI telemetry.

A token expires when the peer session unregisters, the lease changes, the agent restarts, or a bounded API expiry is reached. Resolving a token never falls back to alias, address, most-recent peer, or current lease.

The core application operation accepts the already-resolved `LeaseRevocationRequest`. Issue #13 owns token issuance and resolution at the transport boundary.

## Mutation operation

The transport-neutral operation name is `agent.control.revoke`.

Request:

```json
{
  "schemaVersion": "krasis.agent.control-revoke/v1",
  "requestId": "bounded caller-generated idempotency key",
  "targetToken": "opaque token from the active-control projection",
  "action": "REVOKE_LEASE|DISCONNECT_PEER|REVOKE_TRUST"
}
```

The adapter rejects unknown fields, trailing JSON, missing or oversized values, an expired token, an unsupported schema, and a request body above its fixed limit before calling the application operation.

Internal resolution produces:

```text
request ID
+ authenticated peer certificate SHA-256 and role
+ exact session ID, lease ID, and epoch
+ requested action
```

The operation is accepted only when the resolved target still equals the currently registered runtime identity. Token resolution and runtime verification are separate checks so a stale token cannot target a reconnect.

Response:

```json
{
  "schemaVersion": "krasis.agent.control-revoke-result/v1",
  "status": "COMPLETED|REJECTED|FAILED",
  "code": "stable_code",
  "replayed": false
}
```

The response contains no fingerprint, target token, session ID, lease ID, epoch, address, path, credential body, raw input, or internal error string.

## Actions

`REVOKE_LEASE` returns the exact active lease to `LOCAL` on this device and sends the exact peer revoke frame. The authenticated peer connection may remain available for a new explicit lease.

`DISCONNECT_PEER` performs the same cleanup and then interrupts the authenticated peer connection. Reconnect requires normal authentication and fresh lease negotiation.

`REVOKE_TRUST` performs local lease cleanup first, persists revocation of the authenticated peer fingerprint, sends the exact revoke frame, and interrupts the connection. A trust-store failure returns `trust_revocation_failed`; it never restores the already revoked lease.

## Idempotency and ordering

`requestId` is required. An exact duplicate while the first request is running waits for that request and receives the same status and code with `replayed=true`. Reusing the ID for different content returns `request_conflict`.

The in-process cache holds at most 64 records. New work is rejected with `request_capacity_exhausted` while every slot is in flight. Completed records may be evicted. Exact peer and lease verification remains the safety boundary after eviction.

Management executes through the peer session write coordinator. Input, heartbeat, lease control, return, local emergency release, and management therefore have one ordering owner. Whichever operation wins is committed; the loser observes the new runtime state and cannot revive or cancel another lease.

## CLI mapping

The product CLI uses the local API only. Its `trust list` and `trust revoke FINGERPRINT` commands are
offline trust-store maintenance and must reject an active peer; they are not substitutes for this exact-active
target-token contract. The commands below remain the planned active-control mapping.

```text
krasis control revoke --active
krasis peer disconnect <alias>
krasis peer revoke <alias>
```

For every command the CLI:

1. Authenticates to the local resident agent.
2. Reads one active-control projection.
3. Verifies that the requested alias, when supplied, equals the projection alias.
4. Keeps `targetToken` in memory and submits it with a fresh request ID.
5. Prints only status, stable code, action, and redacted alias.
6. Returns a nonzero stable exit code for rejected, failed, unauthenticated, forbidden, unavailable, malformed, or timed-out operations.

The CLI never accepts a fingerprint, session ID, lease ID, epoch, trust-store path, or peer address as a command-line argument. It does not retry a rejected stale target against a newly observed lease. A user repeats the command deliberately after reading the new projection.

## Stable application codes

The application codes are:

```text
completed
unauthenticated
forbidden
invalid_request
request_conflict
request_capacity_exhausted
no_active_session
peer_mismatch
lease_mismatch
trust_revocation_failed
local_cleanup_failed
peer_notification_failed
disconnect_failed
request_canceled
request_deadline_exceeded
internal_failure
```

Transport adapters may map these to HTTP, RPC, or process exit status, but the JSON `code` remains unchanged.

## Metrics and diagnostics

The registry exports aggregate counts by stable code and a replay count. Metrics do not include principal, peer, target, lease, time series of user activity, address, path, or request body.

A support bundle may report that a management action failed with a stable code and release revision. It may not contain the operation target or enough timing and input detail to reconstruct user activity.
