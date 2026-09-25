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

Unauthenticated or already-revoked peers cannot start an application session or active lease. Session, lease, epoch, sequence, and live trust authorization are checked at use time. The local trust API requires an active session to disconnect before revocation; exact active-lease revocation is a separate management operation. An existing pinned session may use its last successful trust read for at most five seconds when a subsequent filesystem read stalls; a definite revocation or trust error stops it on the next check.

Malformed, duplicated, stale, out-of-order, or outcome-unknown input is dropped and escalated to cleanup. Krasis does not guess delivery or replay uncertain input. Local emergency release and Windows lifecycle termination revoke control and attempt `ReleaseAll`.

Synthesized input must not loop back to a peer. Tagging is diagnostic metadata; classification also uses Windows injected flags and current session direction.

Credentials, trust records, reports, launcher configuration, and stop markers use the handle-relative mutation boundary in `docs/security/local-write-inventory.md`. Reparse traversal, hard-linked rename sources, destination precreation, ACL drift, and parent path replacement fail closed. Credentials and trust records additionally use restrictive handle-validated DACLs, bounded parsing, locking, atomic replacement, and explicit revocation.

The listener uses TLS 1.3 mutual authentication, ALPN, a four-handshake pre-auth concurrency gate, five-second deadlines, a global token bucket, a per-source token bucket, and a fixed-size idle-expiring source table. Source addresses stay ephemeral and diagnostics expose only aggregate rejection causes. The accepted listener policy is `trusted-private`; unspecified or public policies fail before binding. Public-profile support requires a separate reversible Windows firewall implementation.

The Phase 0 crash guard runs with the same user and privilege, inherits only its anonymous pipe, accepts a bounded versioned protocol, stores no input content, and releases known tokens on parent failure.

## Residual risks

Runtime local-data mutation no longer follows a pathname after acquiring its target directory. A malicious process already running as the same user may still read that user's files, inject into the process, abuse a debugger, or replace unsigned executables; the secure-write boundary is not a same-account sandbox. Signed installation and executable integrity remain owned by issue #16.

Power loss can prevent cleanup from running. Windows secure desktops and higher-integrity targets are outside the MVP guarantee. Laptop-specific drivers and touchpad conversion require physical evidence. Local resource exhaustion, firewall misconfiguration, certificate lifecycle mistakes, and compromised trusted peers remain release blockers until their owning issues are accepted.

The attended credential UI probe is an experimental, elevated one-shot command. Its possible side effect is closing the foreground PIN prompt with Escape. It must positively identify the broker before injection, never observe credential contents, never start a peer session, and never be interpreted as evidence that remote keyboard or mouse input works in a protected dialog.

## Privacy boundary

Never collect or emit raw key events, typed text, pointer trajectories, clipboard or screen content, private keys, certificate bodies, local usernames, absolute paths, IP addresses, hostnames, device identifiers, window handles, thread identifiers, or unrestricted command lines.

Only bounded classifications, counts, durations, version identifiers, and revision-addressed artifact identity approved by a report schema may enter diagnostics.
