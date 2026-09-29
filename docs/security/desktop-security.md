# Desktop security boundary

- Status: Active policy; product release remains blocked
- Owner: repository maintainers
- Product contract: `docs/product/02-spec.md`
- Threat model: `docs/engineering/08-threat-model.md`

## Supported security boundary

Krasis is a same-user, direct peer-input product for explicitly paired Windows devices. It is not a remote administration tool, privileged helper, cloud relay, login-screen controller, or security-desktop input provider.

The resident agent, UI, CLI, launcher, and crash guard run without elevation in the current interactive session. They must never request elevation merely to broaden input reach. Control of a higher-integrity application is not a supported guarantee, and Windows sign-in, UAC secure desktop, lock screen, Session 0, and another user's desktop are excluded.

An attended, one-shot `credential-ui-probe` experiment is the sole exception to the no-elevation rule. The destination device's local tray menu can request UAC for this one-shot command; the user must approve locally. It exits before resident startup, identifies only a foreground `CredentialUIBroker.exe` on the Default desktop, and sends a single Escape press/release. It does not read credential content or enable elevated peer input. A successful observation does not change the supported product boundary.

An independent UIAccess probe is also allowed as an attended experiment. It
requires a separately trusted code signature and an administrator-owned
installation location. Generations 91 through 94 may deliver the test executable and
public certificate through peer update, but neither is installed, trusted, or
run by that update. The probe sends only one Escape pair after
positive broker identification. It is not a resident privileged input path.

## Input safety

The destination must prepare before source suppression starts. Exact session, lease, epoch, and sequence identity govern every active-control transition. Disconnect, timeout, lifecycle termination, injection uncertainty, stale traffic, local emergency release, or component failure revokes control, disables suppression, attempts `ReleaseAll`, and converges to `LOCAL` or an explicit fault state.

A GUI close is not evidence that input cleanup completed. Product shutdown, rollback, uninstall, repair, and upgrade must use the resident agent's authenticated cleanup path and verify the final local state.

## Credential and trust boundary

Phase 0 credentials remain short-lived experiment assets. Product identity and pairing follow ADR 0035:
an ACL-protected installation identity, 5-minute transcript, the same human-verified code on both peers,
and signed confirmation receipts from both devices. Trust is created only after both receipts verify.
Private keys and certificate bodies never enter UI-facing output.

Identity rotation creates a new generation; peer replacement atomically revokes the old transport
fingerprint and adds the new one. Expired transport credentials, revoked records, not-yet-valid
certificates, corrupt identity, overly permissive ACL, reparse paths and lock contention fail closed.
Authenticated exact-active-lease revocation is implemented separately by ADR 0034.

A lost or suspected-compromised device is removed from trust on every remaining peer before another session is allowed. An already active lease must be terminated, held input released, local suppression disabled, and the affected identity replaced rather than merely renamed.

## Installation and executable integrity

The probe kit is not a product release artifact. A limited beta requires signed binaries and installer, checksum and SBOM publication, exact commit provenance, minimal reversible firewall changes, safe repair and uninstall, and no bundled credentials, reports, LAN addresses, or user data. Issue #16 owns the implementation and evidence.

Sensitive runtime writes follow `docs/security/local-write-inventory.md`: opened directory handles bind mutation identity, reparse traversal and hard-linked sources fail closed, and credential/trust DACLs are validated through the same handles. Executable replacement remains a signed-installer and launch-integrity concern owned by issue #16.

## Local data and diagnostics

Credentials, trust records, configuration, logs, caches, and diagnostic bundles are separate data classes with separate deletion choices. Diagnostics remain local and aggregate-only by default. There is no automatic upload, telemetry backend, input-content logging, or silent support-bundle collection.

Raw key values, typed text, pointer trajectories, clipboard or screen content, private keys, certificate bodies, usernames, absolute paths, IP addresses, hostnames, device serials, window handles, thread identifiers, and unrestricted command lines are forbidden in product diagnostics and incident reports.

## Network exposure

TLS 1.3 mutual authentication, ALPN, certificate role, validity, and live trust authorization remain independent gates. Pre-auth resource controls do not turn a public or shared network into a supported environment.

Until listener admission and reversible firewall policy are accepted, run only on a user-controlled private LAN or personal hotspot. Do not disable Windows Firewall or add a broad inbound rule to make a peer reachable.

## Release blockers

A release is blocked by any authentication bypass, stale or duplicated control owner, unbounded pre-auth resource path, failure to release local or remote input, secret exposure, sensitive diagnostic field, unsigned executable, unsafe local write, unsupported elevation promise, or missing four-device physical evidence.
