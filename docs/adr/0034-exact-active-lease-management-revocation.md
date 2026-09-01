# ADR 0034: Authenticated exact active-lease management revocation

- Status: Accepted
- Date: 2026-09-01
- Owners: resident-agent, trust, and peer-session maintainers

## Context

Live trust authorization prevents a revoked peer from receiving a later lease. It does not terminate a lease that was authorized before the trust record changed. A product device-management action therefore needs to stop current control immediately without granting the local management caller arbitrary peer or input authority.

The active control identity is not a device alias or a socket. It is the authenticated peer certificate identity together with the exact session, lease, and epoch currently owned by the runtime. Reconnect creates a new session identity, and a delayed request for an older connection must not affect it.

Input, heartbeat, return, lease control, and failure cleanup already serialize through the session write coordinator and `application.SessionRuntime`. A management implementation that owns a second state machine or closes a socket without first cleaning the runtime would create competing owners and ambiguous held-input state.

## Considered approaches

### Trust-store reload only

This is sufficient for future authorization but leaves the current lease active until normal teardown. It does not meet immediate revocation.

### Close the peer socket

Closing the connection eventually drives failure cleanup, but it does not prove that the caller targeted the current peer and lease. It also loses the opportunity to release local state before transport teardown and cannot distinguish duplicate or stale management requests.

### Add a separate management state machine

A second state machine could track device actions independently. It would race the runtime's lease, sequence, liveness, pressed-input, and suppression owners and is rejected.

### Bind a management command to the existing session owner

The local authenticated management adapter supplies a capability-bearing principal and an exact target. The active session registry binds that request to the currently registered peer session. The operation executes through the existing session write coordinator and calls the existing runtime revocation transition. This reuses one ownership and cleanup path.

## Decision

Use the existing session owner.

A revocation request contains:

| Field | Contract |
| --- | --- |
| Request ID | Non-empty, bounded idempotency key scoped to the resident agent process |
| Action | `REVOKE_LEASE`, `DISCONNECT_PEER`, or `REVOKE_TRUST` |
| Peer | Authenticated certificate SHA-256 identity and certificate role |
| Lease | Exact session ID, lease ID, and epoch |

The local service transport authenticates a same-user management client and constructs a `ManagementPrincipal` with the `LEASE_REVOKE` capability. Unauthenticated or authenticated-but-unauthorized callers cannot reach the active session operation. The Phase 0 lab command does not mint this principal.

The resident agent owns one bounded `SessionRegistry`. It retains at most 64 request records. Concurrent repetitions of the same request ID and identical payload execute once and replay the recorded aggregate result. Reusing a request ID with different content is rejected. Capacity exhaustion rejects new work rather than dropping an in-flight request.

Each peer-session registration has a monotonically increasing in-process generation. Teardown from an older connection cannot unregister a newer reconnect. No runtime lease, request cache, or generation is restored across process restart.

Before any side effect, the registered TLS peer identity must equal the request peer identity, and the runtime snapshot must expose the exact requested session, lease, and epoch as `INBOUND_ACTIVE` or `OUTBOUND_ACTIVE`. A missing, stale, reordered, duplicated, or already-cleaned target is rejected without changing suppression, trust, transport, or input state.

The accepted execution order is:

1. Enter the existing session write coordinator so management serializes with input, heartbeat, lease control, return, and failure cleanup.
2. Recheck authenticated peer and exact active lease identity.
3. Disable the source activation owner so no new suppressed outbound input is accepted.
4. Call `SessionRuntime.RevokePeer` for the exact identity. This releases held destination input and converges local control to `LOCAL` or an explicit fault.
5. For `REVOKE_TRUST`, persist revocation of the authenticated certificate fingerprint. Local input cleanup is not delayed behind trust-store I/O.
6. Send the existing exact `ControlLeaseRevoke` frame to the peer.
7. For disconnect and trust actions, interrupt the connection after notification. If notification fails, interrupt it immediately so the peer's authenticated session reaches its existing EOF, timeout, and cleanup path.

A peer-notification failure cannot roll local state back into active control. A trust-store failure is reported, the local lease remains revoked, and the connection is interrupted so the failed trust mutation cannot be mistaken for a complete device removal.

The service returns only status, stable code, and replay classification. Aggregate metrics count outcomes and replays. Errors and metrics do not contain the management subject, fingerprint, session ID, lease ID, epoch, address, credential material, raw input, or local path.

## Stable result codes

The application boundary defines `completed`, `unauthenticated`, `forbidden`, `invalid_request`, `request_conflict`, `request_capacity_exhausted`, `no_active_session`, `peer_mismatch`, `lease_mismatch`, `trust_revocation_failed`, `local_cleanup_failed`, `peer_notification_failed`, `disconnect_failed`, `request_canceled`, `request_deadline_exceeded`, and `internal_failure`.

Adapters may map these codes to transport-specific status codes, but they must not replace them with unbounded internal error text.

## Service and CLI integration

Issue #13 owns the resident agent and authenticated local transport. It must create one `SessionRegistry`, pass it and the trust-store adapter through `SupervisorDependencies`, and map an authenticated local request to `SessionRegistry.Revoke`.

Issue #14 owns the product CLI. Its revoke and disconnect commands must first read the agent's redacted active-peer projection, then submit the exact opaque peer and lease selectors returned by the agent. The CLI never fabricates a lease identity from an alias and never edits the trust store directly.

The normative request, response, authorization, and CLI mapping are in `docs/agent/management-contract.md`.

## Consequences

A management request can be rejected as stale after another cleanup wins the coordinator. This is correct and does not retry against a newer lease.

A bounded request cache means a very old request ID can be forgotten after completed entries are evicted. Safety still depends on the exact active peer and lease check, so replay after eviction cannot cancel a different session.

Immediate revocation relies on local runtime cleanup. When the peer is unreachable, transport interruption and the existing peer timeout are the remote convergence mechanisms; the local device does not remain suppressed while waiting for remote acknowledgement.

## Validation

Deterministic tests cover caller authorization, duplicate replay, request-ID conflict, bounded in-flight capacity, reconnect generation, exact inbound lease revocation, different-peer rejection, stale-lease rejection, peer-notification failure, concurrent trust revocation, coordinator contention representing input and heartbeat work, and a late request after local emergency release.
