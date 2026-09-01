# Limited-beta operations and incident severity

- Status: Active policy; no beta cohort is currently open
- Primary owner: release owner named in the release manifest
- Escalation owner: the same release owner until a second maintainer is explicitly named
- Release policy: `docs/ops/release.md`

## Service commitment

Krasis has no uptime SLA and no continuous remote-control guarantee. A limited beta operates only during the support window written in its release manifest. Outside that window, a tester's safe action is to stop Krasis and return both devices to local control rather than depend on maintainer availability.

If the release owner is unreachable during a safety or security incident, the beta remains disabled. Absence of a backup owner is not permission to continue input transfer.

## Severity

| Severity | Definition | Required action |
| --- | --- | --- |
| SEV-0 | Local input is trapped, suppression remains active, remote input is held, unauthorized control occurs, a stale lease revives, or a device/signing private key is exposed | Emergency release, stop both peers, revoke affected identity, halt candidate distribution immediately |
| SEV-1 | Cleanup, rollback, installer, trust, active-revocation, listener availability, or diagnostic privacy fails without a current trapped-input condition | Disable the affected candidate or feature, preserve redacted evidence, decide rollback before another live session |
| SEV-2 | Supported input, lifecycle, reconnect, compatibility, latency, or soak behavior is materially degraded but safe local recovery works | Record regression, restrict the support table, schedule a fix before expanding the cohort |
| SEV-3 | UI, documentation, diagnostics presentation, or non-safety workflow defect | Track normally; do not weaken a safety or privacy gate to work around it |

## First ten minutes

For SEV-0 and SEV-1:

1. Restore local control and verify both peers are `LOCAL` or stopped.
2. Stop the candidate on both devices and prevent startup.
3. Revoke a suspected peer or identity and isolate the candidate artifact by digest.
4. Record the exact release manifest, time, severity, affected boundary, and aggregate failure code.
5. Do not collect raw input, screen content, credentials, addresses, usernames, local paths, or unrestricted process data.
6. Decide whether the previous signed candidate is safe to restore under `docs/ops/rollback.md`.

## Response targets

During the declared beta support window, the operational targets are:

| Severity | Acknowledge | Containment decision | Written follow-up |
| --- | --- | --- | --- |
| SEV-0 | 1 hour | 2 hours | 2 business days |
| SEV-1 | 4 hours | Same support day | 3 business days |
| SEV-2 | 1 business day | Before cohort expansion | Next release notes |
| SEV-3 | 3 business days | Normal backlog | When fixed |

These are beta operating targets, not contractual availability promises. Missing a SEV-0 or SEV-1 target keeps the cohort disabled until ownership is restored.

## Four-device regression standard

Every limited-beta candidate uses one clean commit and one evidence policy across H-A, H-B, S-A, and S-B. The set must include:

1. Complete redacted device inventory and baseline diagnostics for all four devices.
2. Basic, guided, pointer-shadow, wheel-surface, and touchpad-surface probes where applicable.
3. Peer session and repeated boundary round trips in both directions for each pair.
4. Keyboard, buttons, drag, pointer movement, vertical and horizontal touchpad scroll, and physical mouse wheel behavior.
5. Menu, mouse-capture, pressed-input, move/size, queue, disposition, two-phase entry, focus fallback, lifecycle, and crash-guard evidence.
6. Destination and source termination, network interruption, fresh reconnect, lock, suspend or standby, display change, guard parent EOF, and uncertain injection cleanup.
7. DPI, scaling, privilege, security-software, polling-rate, latency, queue, gap, recovery, and issue #7 approved soak thresholds.
8. Strict checker success with identical build revision and policy, no hand-edited pass, no contradictory totals, and no sensitive fields.

A hosted runner, mock adapter, copied report, or one successful direction cannot replace this standard.

## Beta entry and exit

The initial cohort is limited to the four target devices. Entry requires every release gate in `docs/ops/release.md`, a named support window, a rollback candidate, and no open SEV-0 or SEV-1 incident.

The cohort is suspended immediately by any SEV-0, repeated SEV-1, provenance mismatch, failed four-device regression, or loss of release ownership. Restart requires a new signed candidate and regression covering the incident.

A candidate exits beta only by withdrawal or an explicit promotion decision. Promotion requires all limited-beta blockers closed, the approved soak and incident-free criteria recorded in the final Phase 0 and beta evidence, deletion and rollback rehearsed, and support ownership sized for the next cohort. Passage of time alone does not promote a build.

## Post-incident record

The release owner records timeline, affected contract, root cause, safety outcome, credential action, rollback decision, tests added, release impact, and prevention. The report stays aggregate and redacted. Follow-up issues receive an owner and release-blocker classification before the cohort restarts.
