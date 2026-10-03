# Network exposure and listener recovery

- Status: Active
- Owner: resident-agent and release maintainers
- Decision: `docs/adr/0033-hybrid-pre-auth-admission.md`

## Supported operating boundary

Run the Krasis listener only on a trusted private LAN controlled by the user or on a personal hotspot. The current listener policy does not support a public Windows network profile, an untrusted shared LAN, or a network whose membership cannot be determined.

The launcher binds the listener to the configured literal local IP, never to `0.0.0.0` or `::`. The configured IP must belong to the listener device on the selected private interface. From g122, a running listener follows a unique replacement private IPv4 address on that same interface, and the connector can discover and authenticate the new endpoint before saving it. An unchanged address does not trigger a listener rebind. Unavailable or ambiguous addresses, adapter replacement and unsupported routing still require operator recovery. See [LAN address recovery](../desktop/address-recovery.md) for the exact boundary and device-test requirements.

Public Wi-Fi can isolate clients even when both devices appear connected. Do not compensate by opening a broad inbound rule, disabling the firewall, binding an unauthenticated listener, or weakening mTLS and trust authorization.

## Listener controls

Before TLS, Krasis applies a bounded global token bucket, a bounded per-source token bucket, a fixed-size idle-expiring address table, a four-handshake concurrency gate, and a five-second handshake deadline.

After admission, TLS 1.3 mutual authentication, ALPN, certificate role, certificate validity, and live trust-store authorization still apply independently. A passed limiter is not a trusted peer.

Only aggregate counters are exposed:

- admitted pre-auth handshakes
- concurrency saturation
- invalid remote-address rejection
- source-table saturation
- per-source rate rejection
- global rate rejection
- TLS authentication failure
- authenticated peer completion

Peer addresses are never exported or persisted by this metric surface.

## Public or shared network procedure

The current product decision is to refuse public-network listener operation. Use a personal hotspot or another user-controlled private LAN instead.

A future public-profile implementation must show the selected executable, local port, Windows profile, allowed peer policy, creation result, and removal action before enabling the listener. It must be reversible without deleting unrelated firewall rules.

## Recovery when connectivity is blocked

1. Stop the Krasis listener so no active lease remains.
2. Confirm both devices are on the intended private network and can reach each other without disabling endpoint security.
3. Inspect only the firewall rule scoped to the Krasis executable and selected port. Do not enable a blanket inbound exception.
4. Remove or disable that exact rule if it is stale, then restore the previous firewall state.
5. Restart the listener and confirm mTLS authentication and `LOCAL` state before attempting control transfer.
6. If rejection counters rise, wait for the bounded refill window instead of increasing limits during an incident.

No rollback step may preserve an active remote lease or leave local input suppressed.
