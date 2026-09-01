# ADR 0033: Hybrid pre-auth admission and trusted-private listener policy

- Status: Accepted
- Date: 2026-09-01
- Owners: transport and resident-agent maintainers

## Context

TLS 1.3 mutual authentication, ALPN, certificate role, and live trust authorization prevent an unauthenticated socket from becoming a Krasis session. They do not prevent slow or repeated handshakes from consuming accept, goroutine, TLS, and certificate-verification capacity.

The listener already limits concurrent pre-auth handshakes to four and gives each handshake five seconds. A single source could still reconnect continuously, while an attacker rotating source addresses could bypass a per-address-only limiter. Any address tracking must have fixed memory and must not enter diagnostics.

The current product boundary supports a trusted private LAN or personal hotspot. Krasis has no Windows network-profile owner or reversible firewall-rule manager yet.

## Considered approaches

### Per-source limiter only

This blocks one noisy address and preserves capacity for other peers. It fails against address rotation, and NAT can place multiple valid devices behind one address.

### Global token bucket only

This has constant memory and survives address rotation. One noisy source can consume the entire budget and delay a valid peer.

### Firewall-only admission

A correctly scoped Windows firewall rule rejects traffic before TLS work. Krasis cannot currently verify, create, remove, or recover such a rule safely, and public Wi-Fi client isolation can still prevent legitimate peer traffic.

### Hybrid listener admission plus firewall policy

A global bucket bounds total handshake starts. A per-source bucket contains one source. A fixed-size, idle-expiring source table bounds memory. The existing concurrency gate and handshake deadline remain independent. Public-network support remains disabled until a product-owned firewall and network-profile contract exists.

## Decision

Use the hybrid design.

The default listener admission values are:

| Control | Value |
| --- | --- |
| Concurrent pre-auth handshakes | 4 |
| Handshake deadline | 5 seconds |
| Global token bucket | 16 tokens/second, burst 32 |
| Per-source token bucket | 4 tokens/second, burst 8 |
| Maximum tracked source addresses | 256 |
| Source idle expiry | 2 minutes |

Source identity is a normalized IPv4 or IPv6 address without a port. IPv4-mapped IPv6 addresses collapse to the same key. The address table exists only in memory and is never exported, logged, or persisted.

A new source is rejected when the bounded table is full and no idle entry can be removed. Per-source admission is checked before global admission so one noisy source cannot spend global tokens faster than its own quota. Rotating sources still share the global bucket.

Clock rollback does not refill tokens. Invalid remote addresses fail closed. Rejection closes the socket before TLS authentication.

`ListenSource` exposes only aggregate counters for admitted handshakes, concurrency saturation, invalid source addresses, source-table saturation, per-source rate rejection, global rate rejection, failed TLS authentication, and authenticated peers.

The Phase 0 compatibility constructor asserts `trusted-private`. Production code must use the explicit policy constructor. The only accepted policy is currently `trusted-private`; unspecified or public policies fail before a socket is bound. Supporting a public profile requires a separate accepted ADR and a reversible firewall implementation scoped to executable, port, Windows profile, and peer policy.

mTLS, ALPN, certificate role, certificate validity, and live trust-store authorization remain independent gates after admission. Admission success never grants session or lease authority.

## Consequences

A NAT containing many reconnecting valid peers shares one per-source burst and can receive temporary rate rejection. The burst of eight intentionally favors bounded recovery over unlimited reconnect storms.

A DHCP address change creates a new ephemeral entry. Idle expiry and the global bucket prevent unbounded growth or address-rotation bypass.

The limiter improves listener availability but does not make public networks supported. Operators must use a trusted private LAN or personal hotspot and follow `docs/ops/network-exposure.md`.

## Validation

Deterministic tests cover per-source exhaustion and refill, rotating-address global exhaustion, bounded table capacity and idle eviction, IPv4-mapped normalization, invalid configuration, clock rollback, aggregate metrics, an incomplete TLS handshake concurrent with a valid authenticated peer, and explicit network-policy validation.
