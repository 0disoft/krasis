# Local data and diagnostic retention

- Status: Active policy; identity, trust and protected input paths are implemented; complete uninstall and selective cleanup remain in #16
- Owner: release owner named in the release manifest
- Trust decision: `docs/adr/0006-persistent-trust-store.md`
- Secret policy: `docs/ops/secrets.md`

## Data classes

Krasis keeps product configuration, device identity, peer trust, diagnostics, logs, caches, and release metadata as separate classes. A UI, CLI, installer, repair, rollback, or uninstall operation must not collapse them into one opaque data directory or one all-or-nothing deletion choice.

The current-user product data root is `%LOCALAPPDATA%\Krasis`. Device identity is stored under
`identity`, peer trust under `trust`, CLI config in `config.json`, local API discovery credentials under `control`, and diagnostics
under `diagnostics`; these classes are not
interchangeable. The attended input installer owns the fixed system Program Files `Krasis\Input`
location and `KrasisInput` service (ADR 0066). It does not remove user identity or trust data.
Issue #16 still owns full-app install/startup policy, firewall ownership, complete uninstall and
selective cleanup. Limited beta remains blocked until those remaining contracts are implemented and tested.

The product root and identity data allow only the current user and `SYSTEM`. Existing permissive or
reparse-backed roots fail closed instead of being silently repaired. Device identity uses
`identity\identity.json` plus `.identity.lock`; peer trust keeps its existing `trust.json` and
`.trust.lock` contract below the product trust directory.

The control directory contains a per-process loopback endpoint document and 256-bit bearer token.
Both are deleted at agent shutdown and are never included in diagnostics, export, backup or migration.

## Product retention policy

| Class | Contents | Default retention | Deletion behavior |
| --- | --- | --- | --- |
| Configuration | Peer layout, preferences, bounded product settings | Until changed or product data is explicitly deleted | May be deleted without exporting secrets; next start uses safe defaults and no active lease |
| Device identity | Private key and public certificate | Until rotated, revoked, or product data is explicitly deleted | Terminate active control, guide revocation on peers, then delete the protected identity directory |
| Peer trust | Fingerprint, alias, role, capabilities, revocation state | Until user revokes or product data is explicitly deleted | Revocation is persisted; deletion does not imply the remote peer forgot this device |
| Diagnostics | Aggregate schema, release identity, bounded counters, durations, classifications and failure codes | Seven days or twenty bundles, whichever limit is reached first | Oldest bundle is removed first; user can delete all immediately |
| Operational logs | Aggregate lifecycle, startup, shutdown and failure classifications | Seven days with a bounded byte limit defined before beta | Rotation deletes oldest records; no input content or peer address is allowed |
| Cache | Rebuildable UI, update-check or temporary product data | At most twenty-four hours unless a shorter owner contract applies | Safe to remove while the agent is stopped |
| Release metadata | Installed version, commit, signature and checksum identity | Current and previous rollback candidate | Removed with binaries after rollback evidence is no longer needed |

Retention is implemented locally. There is no background upload or cloud retention.

## Diagnostic fields

Product diagnostics may contain only allow-listed fields needed to prove safety and recovery:

1. Schema version, product version, clean build revision, target architecture, and bounded Windows edition/build classification.
2. User-selected redacted device alias or role, never serial number, hostname, account name, or stable hardware identifier.
3. Requested and observed durations, count totals, latency buckets, queue/gap totals, lifecycle category, crash-guard category, stop reason, stable failure code, and pass/fail state.
4. Aggregate input-kind counts without key codes, text, absolute coordinates, pointer paths, application names, window data, or timestamps precise enough to reconstruct user activity.
5. Signature, checksum, report-schema, and policy identifiers needed to establish artifact provenance.

Private keys, certificate bodies, trust fingerprints, session or lease identifiers, IP or MAC addresses, ports, usernames, absolute paths, command lines, environment variables, clipboard, screen content, raw input, and device serials are forbidden.

## Export and upload

Krasis never uploads diagnostics automatically. Export is an explicit per-bundle action. Before writing the bundle, the UI or CLI shows the field classes, retention consequence, destination, and that the user must transmit it separately.

An exported bundle is a new user-owned copy and is not silently deleted by local rotation. It contains aggregate JSON and release metadata only. It never includes the live data directory, arbitrary logs, credentials, config with LAN addresses, screenshots, or a recursive file archive.

## Deletion and uninstall

Uninstall first stops the agent, revokes active control, disables suppression, attempts `ReleaseAll`, removes Krasis-owned startup and firewall entries, and removes binaries. It then presents separate choices for configuration, identity and trust, diagnostics and logs, and cache.

Deleting identity or trust data while another device may still trust it requires a warning and peer-revocation guidance. Secure erasure on SSD storage is not promised; revocation, key rotation, protected ACLs, and ordinary file deletion are the supported controls.

A failed or interrupted cleanup reports the remaining class without exposing its private path. It never reports success after a partial deletion.

## Phase 0 data

Phase 0 remains governed by explicit user-selected directories. Its launcher config contains role, redacted alias, peer address, port, edge, duration, and relative credential/report directories but no private key or report body. It is not a product config format.

Phase 0 reports are local aggregate evidence and are not automatically deleted because an experiment owner may need them for revision-addressed acceptance. Phase 0 credential directories are deleted manually on both devices after the experiment and required evidence review. They are never migrated silently into product identity.

Repository build caches, `build/`, `dist/`, downloaded workflow artifacts, and local smoke reports are generated developer output, not product data or source truth.

## Recovery

Corrupt, unknown-schema, overly permissive, reparse-backed, or lock-contended identity and trust data fails closed. It is not converted into an empty trusted set or a successful reset.

A reconnect or process restart never restores an active lease, input sequence, liveness deadline, pressed-input journal, suppression state, or cursor snapshot from disk.
