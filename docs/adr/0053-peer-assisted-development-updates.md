# Peer-assisted development updates

Status: accepted

## Scope and authorization

The operator requested one-click transfer and application of a development build
to a paired laptop after one local opt-in. Input-control permission alone does
not grant update permission. Opt-in must bind the exact paired certificate, be
revocable locally, and be rechecked before receiving and before applying a bundle.
The existing unsigned development ZIP is not an official signed release. The
opt-in delegates code supply to that paired computer; hashes only detect damage.
No generic remote shell, arbitrary path write, or arbitrary file execution API
is part of this feature.

## Delivery stages

1. Validate and snapshot the complete distribution.
2. Transfer over a separately bounded, mutually authenticated connection so file
   traffic does not hold the input-session write lock.
3. Verify fixed file names, total and per-file size, SHA-256 checksums, platform,
   clean Go build identity, matching revision, and a higher agent generation.
4. Stage in a new user-private directory; never replace the running executables.
5. Recheck permission, drain the input connection, launch the new GUI, and verify
   the expected agent build before reconnecting. Downloaded is not installed.

The first supporting version must be installed manually on both PCs. Unknown or
old peers must not receive a new input protocol message. Retained old directories
provide a manual recovery path; automatic downgrade is not authorized.

## Increment 1

`internal/updatebundle` implements bounded ZIP validation, distribution packing,
and fresh-directory staging. It never executes received code.

## Transfer and local API contract

Generation 21 uses HTTP/1.1 over the existing pinned mutual TLS roles on the input
listener's port plus one. The listener serves an explicitly queued bundle; the
connector polls every two seconds only while authorized, or uploads its queued
bundle. TLS 1.3, certificate expiry/revocation, current CONNECTED peer identity,
and per-device opt-in are checked separately. Input protocol v1 is unchanged.
Transfers have a 90-second limit, offers expire after three minutes, and only one
bundle is admitted at a time. There is no blind retry of an ambiguous upload.

The authenticated loopback API adds POST `/v1/update` with schema
`krasis.peer-update/v1`: STATUS (no fields), ALLOW/DENY (exact fingerprint), SEND
(fingerprint plus local distribution directory), APPLY/APPLY_FAILED (fingerprint
plus exact ready job ID). Unknown actions and cross-action fields are rejected.
The LAN API cannot select local paths or grant itself update permission. Permission
is saved under the private product root; changed/revoked certificates do not
inherit it. Before APPLY, trust is rechecked and the job is consumed once under
the permission lock. New pairing/connections are fenced until handoff finishes.

The GUI retains the old distribution, launches the freshly revalidated bundle,
and exits normally. The new GUI waits for old GUI ownership, uses the existing
agent-upgrade handshake, checks the exact expected runtime build, and reconnects.
The sender GUI also prepares its agent and reconnects after the receiver's stop.
Local launch failure clears the apply fence. A new-process crash/startup failure
requires reopening the preserved old distribution; automatic rollback is not
claimed. Permissions survive restart; pending transfers deliberately do not.
The receiver GUI must be open or in the tray. Received files are never executed
solely by the network handler. Fixed stage codes omit paths, contents and aliases.

## Verification boundary

Unit tests cover denied and revoked permissions, single-use apply, malformed ZIPs,
generation checks and integrity checks. Packaged two-PC upgrade, firewall behavior,
native GUI relaunch and post-upgrade input require manual verification. No official
publisher signature, production feed, global rollout or release is claimed.
