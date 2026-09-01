# Rollback and emergency disable

- Status: Active policy
- Primary owner: release owner named in the release manifest
- Incident policy: `docs/ops/service-levels.md`

## Rollback triggers

Rollback begins immediately for a safety or security incident, failed cleanup, stale active lease, held input, unreleased suppression, unauthorized peer control, credential compromise, corrupted local data, signature or checksum mismatch, installer or repair failure, or a regression that cannot be bounded to an unsupported feature.

Compatibility and performance defects trigger rollback when disabling the affected input path cannot restore the documented safe boundary.

## First action

Preserve local input before preserving diagnostics or application state.

1. Use the local emergency-release action on the affected device.
2. Stop new input transfer and close the peer connection.
3. Verify source suppression is disabled.
4. Attempt `ReleaseAll` on the destination and verify both sides report `LOCAL` or an explicit fault.
5. Stop the resident agent on both devices. Do not restart into the same active session.

When the UI or management API is unavailable, terminate the signed Krasis process through Windows and keep the peer offline. Do not disable input devices, Windows security controls, or the firewall to recover Krasis.

## Emergency disable

After local input is safe:

1. Disable Krasis startup for the affected candidate.
2. Revoke the peer identity on every reachable trusted device.
3. Remove only the firewall rule owned by the Krasis installer for that executable, port, and profile.
4. Quarantine the candidate installer and binaries by digest so they cannot be redistributed.
5. Preserve one redacted aggregate diagnostic bundle and the release manifest when safe to do so.

If a private device identity may be exposed, revocation is mandatory and re-pairing uses a new identity. Renaming the peer or deleting a UI entry is not revocation.

## Binary rollback

Rollback uses the exact previous signed candidate named in the release manifest. Verify its signature and SHA-256 digest before installation.

Never replace binaries while an agent owns an active connection, suppression state, crash guard, or held-input journal. Uninstall or repair must first execute the safety shutdown and verify the process has stopped.

No runtime session, lease, epoch, sequence, liveness deadline, pressed-input journal, or cursor-entry snapshot is restored. Reconnect creates a new authenticated session and re-pairing is required when identity or schema compatibility is uncertain.

## Local-data compatibility

Krasis has no product database. Rollback concerns product configuration, device identity, trust records, reports, and caches.

A candidate that changes a persistent schema must declare whether the previous signed candidate can read it. When backward compatibility is not proven, the rollback plan revokes the affected identity, exports only a redacted diagnostic bundle, removes the incompatible product data through the documented deletion path, installs the previous candidate, and performs fresh pairing. It must not coerce a new schema into an older one.

Caches and generated output may be deleted. Private keys, trust records, and reports are never copied into build directories or an installer during rollback.

## Verification

Rollback is complete only when:

1. No Krasis process from the rejected candidate is running.
2. No Krasis-owned startup entry or firewall rule for the rejected candidate remains.
3. Both devices start in `LOCAL` with no suppression or held input.
4. The installed binary and installer match the previous manifest and valid signature.
5. A newly authenticated session starts with a new lease and sequence.
6. Revoked or expired identities remain unable to connect.
7. The incident record contains no raw input, credential material, addresses, usernames, or private paths.

## Forward fix

A forward fix requires a new commit, signed immutable candidate, focused regression for the trigger, the full required Windows check, and every release gate affected by the incident. Reusing the rejected version or replacing its bytes is forbidden.
