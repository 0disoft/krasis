# Windows support policy

- Status: Active policy; product support is not yet open
- Owner: release owner named in the release manifest
- Product boundary: `docs/product/02-spec.md`
- Hardware gate: `docs/experiments/phase-0-windows-input-feasibility.md`

## Current support decision

Krasis has no generally supported Windows edition or build today. Phase 0 remains incomplete, the four-device ledger is not complete, and a CI-green Windows build is not hardware support evidence.

The first limited-beta candidate family is Windows 11 on amd64 with one active display per device. An edition and build become supported only when the exact release revision passes the four-device regression and the release manifest lists the observed edition, build, architecture, input-device class, scaling, privilege boundary, and accepted limitations. The current allow-list is empty until issues #4 through #8 are completed.

Windows 10, Windows on ARM64, x86, multi-monitor layouts, virtual machines, remote desktop sessions, Windows Server, and non-Windows systems are outside the current limited-beta policy. They require a separate evidence set and an explicit policy change rather than inference from compilation.

## User and privilege boundary

Krasis runs in the signed-in interactive user's session. The source agent, destination agent, crash guard, tray UI, and local management client must not require administrator elevation for normal operation.

The supported control boundary is applications at the same or lower Windows integrity level as Krasis. Input into an elevated application from a non-elevated Krasis process is not guaranteed. Running Krasis elevated to work around this limitation is unsupported because it expands the impact of peer or local compromise.

Windows sign-in, lock screen, UAC secure desktop, other secure desktops, Session 0, service-owned desktops, and cross-user-session control are excluded. Lifecycle notifications may terminate a session around lock, logoff, suspend, resume, shutdown, or display change, but that does not make secure-desktop input supported.

## Limited-beta allow-list

Every beta release must carry a support table generated from the accepted four-device evidence for that exact clean commit. At minimum it records:

| Field | Required decision |
| --- | --- |
| Windows edition, version, and build | Exact observed values for H-A, H-B, S-A, and S-B |
| CPU architecture | `amd64` for the first candidate; every device must match |
| Display topology | One active display per device |
| DPI and scaling | Exact observed values and any accepted exclusion |
| Input devices | Keyboard, Precision Touchpad, external mouse, wheel and button classes actually exercised |
| Privilege behavior | Standard desktop result, elevated-app result, and secure-desktop exclusion |
| Security software | Redacted product category and any observed interference |
| Standby and lifecycle | Observed capability and passed fault cases |

A device outside the table is unsupported even when its Windows version appears similar. The release owner may expand the table only with a new revision-addressed evidence set.

## Release blockers

Support remains closed while any of the following is true:

1. Phase 0 is `IN_PROGRESS` or issue #8 is open.
2. One of the four target devices lacks a complete inventory or comparable report.
3. A lifecycle, crash recovery, held-input, suppression-release, compatibility, latency, or soak gate is missing or contradictory.
4. The release artifact is unsigned, lacks checksums or an SBOM, or cannot be traced to the exact CI-green commit.
5. The product still relies on Phase 0 credential, launcher, listener, or report paths as a release interface.
6. Installer, rollback, local-data deletion, signing, immediate revocation, or same-user path-race requirements remain release blockers.
