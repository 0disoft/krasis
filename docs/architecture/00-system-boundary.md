# Krasis system boundary

- Status: Active
- Owner: repository maintainers
- Product contract: `docs/product/02-spec.md`

## Repository ownership

This repository owns the peer-input safety model, Windows input adapters, authenticated ordered peer transport, local trust and credential primitives, agent supervision, Phase 0 diagnostics, native probe launcher, evidence validators, and the product contracts that constrain them.

The executable surface includes the bounded `krasis-lab` workflow and launcher plus `krasis-agent.exe`,
which owns one user-session lifecycle and an authenticated loopback management API. Product CLI
configuration, Wails/Svelte tray UI, signed installer and update channel remain later-phase work
tracked by issues and `docs/product/01-roadmap.md`.

## External boundary

Krasis communicates directly between user-owned Windows devices on a local network. The MVP has no cloud account, relay, telemetry backend, clipboard, file transfer, screen capture, audio transfer, or non-Windows runtime.

Windows owns physical input delivery, foreground and lifecycle notifications, credential and file permissions, process execution, and `SendInput`. Krasis must treat every Win32 result whose effect is uncertain as a session fault rather than guessing or replaying input.

## Runtime ownership

`internal/control` is the only transition-rule owner. `internal/application.SessionRuntime` serializes lease state, input sequence approval, pressed-input journals, injection, liveness, and cleanup. Peer, Windows, launcher, and UI adapters request work through those boundaries and do not mutate ownership state directly.

A destination enters `INBOUND_PENDING` before source suppression can start. It becomes `INBOUND_ACTIVE` only after the exact session, lease, and epoch grant is accepted and committed. Disconnect, timeout, sequence uncertainty, local lifecycle termination, injection uncertainty, or cleanup failure revokes the lease, attempts `ReleaseAll`, and converges to `LOCAL` or an explicit fault state.

The detailed sequence and failure behavior are defined in `docs/architecture/02-runtime-flow.md` and accepted ADRs.

## Data ownership

Krasis stores local credentials and trust records only on the owning device. Runtime leases, input sequence, pressed-input state, and pointer state are ephemeral. Diagnostic reports are local, bounded, schema-validated, and aggregate-only. Raw keys, text, pointer trajectories, window identities, credentials, and network identifiers are outside the diagnostic contract.

There is no product database in the current architecture.

## Non-drifting contracts

Changes must preserve the product safety invariants, transition table, authenticated identity binding, exact sequence handling, release-before-local semantics, no stale-lease resurrection, and diagnostic privacy. A change to one of these contracts requires matching deterministic tests and an ADR update or replacement.
