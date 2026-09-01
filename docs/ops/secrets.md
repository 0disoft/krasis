# Secret and credential policy

- Status: Active policy; product identity core is implemented
- Primary owner: release owner named in the release manifest
- Security boundary: `docs/security/desktop-security.md`

## Secret inventory

Krasis secrets are device private keys, any pairing bootstrap secret, code-signing private keys, CI or artifact-store credentials, and future update-signing keys if automatic updates are ever approved.

Certificates, fingerprints, aliases, roles, capabilities, trust records, release manifests, checksums, and SBOMs are not private keys, but they remain integrity-sensitive and must not be silently changed.

## Storage

A product device private key stays in a product-owned Windows directory protected for the current user and `SYSTEM`, with inheritance disabled and reparse-path, file-type, and ACL checks before use. It is never stored in Git, an issue, pull request, log, report, launcher configuration, command line, environment variable, clipboard flow, build artifact, cloud-sync directory, or support bundle.

The product does not copy a device private key between peers. Pairing exchanges authenticated public identity and explicit human confirmation. Replacement devices receive a new identity.

Code-signing keys stay outside the repository and developer worktree. Release automation uses a hardware-backed or managed signing service with the narrowest available permission and short-lived authorization. The release artifact records the signing certificate identity, not private signing material.

CI credentials are repository or environment scoped, unavailable to untrusted pull-request code, and granted only the actions required by the release job. Normal pull-request CI does not receive signing or publication credentials.

## Lifetime and rotation

ADR 0035 defines a non-expiring installation Ed25519 identity that rotates by explicit generation.
Transport certificate validity remains independently enforced, and its product renewal cadence is a
later agent/CLI UX decision. Phase 0 credentials are never migrated into product identity.

Rotation is mandatory when a private key may have been read, a device is lost or transferred, an ACL or reparse check fails, certificate validity is exhausted, a signing certificate changes, a CI credential is exposed, or cryptographic policy changes.

Rotation creates a new key and identity, confirms it on both peers, updates trust atomically, terminates any lease authorized by the old identity, and revokes the old identity before normal operation resumes. An alias edit, certificate renewal without identity binding, or file deletion on one device is not sufficient revocation.

## Lost-device response

1. Stop active control on every reachable peer and verify `LOCAL`.
2. Revoke the lost device fingerprint and capabilities on every remaining device.
3. Block the exact active lease through the authenticated management path when present.
4. Rotate any pairing bootstrap, CI, signing, or shared operational credential that the device could access.
5. Mark the device identity compromised in the incident record without publishing its certificate body, paths, addresses, or hardware identifiers.
6. Require fresh pairing for a recovered or replacement device.

Deleting local data from a missing device cannot be assumed. Revocation on remaining peers is the control that matters.

## Leak response

A suspected secret in Git history, logs, CI output, reports, chat, or an artifact is treated as exposed even after deletion. Revoke or rotate first, remove distribution access, then redact the copy. History rewriting does not restore trust in the old key.

The incident record includes secret class, affected release, detection time, revocation completion, replacement identity, and scope. It never includes the secret value.

## Deletion

Uninstall and data deletion are separate choices. Removing binaries does not silently delete trust records or private keys, because another peer may still trust that identity. The deletion flow first terminates active control, offers peer revocation guidance, then removes the selected product-owned credential directory.

Secure deletion of flash storage is not guaranteed. Revocation, key replacement, protected storage, and ordinary file deletion are the supported controls.
