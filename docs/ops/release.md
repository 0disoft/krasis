# Release policy

- Status: Active policy; releases are blocked
- Primary owner: release owner named in the release manifest
- Incident policy: `docs/ops/service-levels.md`
- Rollback policy: `docs/ops/rollback.md`

## Release classes

g234 adds a self-contained application setup asset alongside the signed ZIP feed.
Local setup candidates are unsigned and cannot be published by sign-release.mjs.
The release environment must configure KRASIS_RELEASE_PUBLIC_KEY and its existing
private-key secret, plus KRASIS_SIGN_CERT_BASE64, KRASIS_SIGN_CERT_PASSWORD,
KRASIS_SIGN_CERT_THUMBPRINT and an HTTPS KRASIS_SIGN_TIMESTAMP_URL. The certificate
is loaded ephemerally, its image signatures and timestamps are checked, and its
temporary CI file is removed even after failure. No certificate is trusted or
installed on customer machines by the build. Configuring and activating publisher
credentials remains a release-owner action, separate from local implementation.

### Phase 0 probe kit

A revision-addressed diagnostic artifact used only by the named target devices and experiment runbook. It is not installed, supported, auto-updated, or represented as a product release.

### Limited-beta candidate

A signed, manually distributed product build restricted to the Windows and device allow-list in `docs/desktop/os-support.md`. Each candidate is immutable and traced to one clean commit.

### Public release

Not authorized by the current product policy. It requires a separate release decision after limited-beta evidence, operational ownership, support capacity, and security blockers are resolved.

## Current status

No limited-beta candidate may be published while Phase 0 is incomplete or issues #4 through #8 remain open. Product identity and pairing, listener hardening, immediate active-lease revocation, resident agent and local API, product CLI, tray UI, signed installer, same-user secure writes, and this operating policy must be accepted before external beta distribution.

## Required merge checks

`main` must be protected by these rules before a limited beta:

1. Changes enter through pull requests.
2. The `ci / windows` check is required.
3. Force pushes and branch deletion are disabled.
4. No reviewer count is required while the project has one maintainer; the required Windows check remains mandatory.
5. An emergency bypass may only disable distribution or remove a known-dangerous artifact. It must not publish feature code, and a normal pull request must record the bypass and replacement revision.

The release owner verifies the live GitHub rule or ruleset before each candidate. Documentation alone is not proof that protection is enabled.

The Windows workflow must pass formatting, `go test ./...`, vet with the documented Win32 exception, both executable builds, probe-kit packaging, archive verification, and artifact upload. A skipped or cancelled required job is not green.

## Limited-beta entry gate

A candidate may enter beta only when all of these are true:

1. The commit is clean, reviewed through a pull request, and the required Windows check passed.
2. The exact commit passes the complete H-A, H-B, S-A, and S-B regression in both control directions under one evidence policy.
3. Phase 0 E0 through E8 outcomes are classified and issue #8 records the final input defaults or accepted exclusions.
4. The lifecycle and crash matrix proves suppression release, held-input cleanup, fresh-session reconnect, and `LOCAL` convergence.
5. Compatibility, latency, queue, heartbeat, timeout, rearm, and soak limits come from issue #7 evidence rather than defaults copied from code.
6. No open Critical risk exists. Every High risk is fixed or accepted in the release manifest with scope, owner, detection, rollback, and expiry.
7. Product pairing, active revocation, resident-agent ownership, management API, CLI, UI, installer, local-data cleanup, listener exposure, and race-resistant sensitive writes passed their acceptance tests.
8. The installer and every executable are signed. Signature verification succeeds on every allowed Windows build.
9. The release bundle includes SHA-256 checksums, an SPDX or CycloneDX SBOM, license notices, build revision, build time, workflow-run identity, supported-device table, known limitations, rollback artifact identity, and deletion instructions.
10. No credential, private key, trust document, report, local path, LAN address, hostname, username, or device identifier is packaged.

## Artifact provenance

The release manifest binds product version, Git commit, clean-tree state, workflow run, compiler version, target architecture, file size, SHA-256 digest, signing certificate identity and validity, SBOM digest, and installer digest.

Rebuilding the same version with different bytes creates a new candidate identifier. An existing candidate is never overwritten. The previous accepted signed candidate and its manifest remain available for rollback until the new candidate exits beta.

## Rollout

Peer-assisted development updates and the publisher-signed GitHub release updater are implemented. Public distribution remains subject to the release decision above. The release owner configures the release environment signing keys, approves an immutable generation tag and manually dispatches the release workflow. The client verifies the signed feed and bundle, downloads in the background and applies only after input and pairing are idle. The release owner confirms both devices are `LOCAL`, verifies signatures and checksums and runs the bounded smoke procedure before external distribution.

The cohort, support window, owner, exact devices, start date, stop date, and rollback revision are recorded in the release manifest.

## Stop conditions

Distribution stops immediately for trapped local input, unreleased suppression, held remote input, unauthorized control, identity or signing-key compromise, stale-lease resurrection, unsafe upgrade or uninstall, sensitive diagnostic leakage, artifact-provenance mismatch, or a reproducible crash that prevents safe cleanup.

A stopped candidate is never silently re-enabled. It requires a new signed revision, a regression covering the incident, and a new release decision.
