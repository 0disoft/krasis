# Krasis threat model

- Status: Active
- Owner: repository maintainers
- Product security baseline: `docs/engineering/04-security-baseline.md`
- Desktop security boundary: `docs/security/desktop-security.md`

## Assets

Krasis protects local input availability, exclusive control ownership, pressed-key and button release, device private keys, trust records, authenticated peer identity, command authorization, executable and probe-kit integrity, and diagnostic privacy.

## Trust boundaries

The main boundaries are Windows physical input to the source adapter, source adapter to the serialized runtime, authenticated peer transport, destination runtime to `SendInput`, local management clients to the future resident agent, local files to credential and trust stores, launcher to adjacent probe binaries, and report writers to user-selected destinations.

The UI, launcher, peer socket, report consumer, and caller-provided paths are not trusted state owners.

## Threats and required controls

Unauthenticated or revoked peers must never obtain an application session or active lease. Session, lease, epoch, sequence, and live trust authorization are checked at use time, not inferred from a previous connection.

Malformed, duplicated, stale, out-of-order, or outcome-unknown input is dropped and escalated to cleanup. Krasis does not guess delivery or replay uncertain input. Local emergency release and Windows lifecycle termination revoke control and attempt `ReleaseAll`.

Synthesized input must not loop back to a peer. Tagging is diagnostic metadata; classification also uses Windows injected flags and current session direction.

Credentials and trust records use restrictive permissions, bounded parsing, locking, atomic replacement, and explicit revocation. Reports use allow-listed schemas, aggregate counters, exclusive creation, path validation, and redaction.

The listener uses TLS 1.3 mutual authentication, ALPN, a bounded pre-auth concurrency gate, and handshake deadlines. Per-source admission, public-LAN firewall automation, and load-tested recovery limits remain unresolved under issue #11; until then the listener is limited to a trusted private LAN or personal hotspot.

The Phase 0 crash guard runs with the same user and privilege, inherits only its anonymous pipe, accepts a bounded versioned protocol, stores no input content, and releases known tokens on parent failure.

## Residual risks

A malicious process running as the same user can race path components or replace executables unless a handle-relative writer and signed installation boundary prevent it. Path-race hardening is tracked by issue #17 and signing by issue #16.

Power loss can prevent cleanup from running. Windows secure desktops and higher-integrity targets are outside the MVP guarantee. Laptop-specific drivers and touchpad conversion require physical evidence. Local resource exhaustion, firewall misconfiguration, certificate lifecycle mistakes, and compromised trusted peers remain release blockers until their owning issues are accepted.

## Privacy boundary

Never collect or emit raw key events, typed text, pointer trajectories, clipboard or screen content, private keys, certificate bodies, local usernames, absolute paths, IP addresses, hostnames, device identifiers, window handles, thread identifiers, or unrestricted command lines.

Only bounded classifications, counts, durations, version identifiers, and revision-addressed artifact identity approved by a report schema may enter diagnostics.
