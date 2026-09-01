# Krasis desktop surface

- Status: Phase 0 launcher implemented; production desktop app pending
- Owner: repository maintainers
- Product contract: `docs/product/02-spec.md`

## Current executable surface

`krasis-launcher.exe` is a native Windows launcher for the two-device Phase 0 probe workflow. It validates its adjacent probe kit, collects bounded configuration, starts `krasis-lab`, and exposes only the controls and reports required by the experiment contract.

It is not the production tray application, installer, updater, or persistent agent owner.

## Planned production boundary

The production desktop surface will project state from the resident agent through an authenticated local API. It may configure pairing, peer layout, connection state, safety state, diagnostics, and explicit disconnect or revoke actions. It must never own the live input queue, lease state machine, heartbeat deadline, pressed-input journal, or `ReleaseAll` cleanup.

The resident agent and local API are tracked by issue #13. The Wails v3 and Svelte tray UI is tracked by issue #15. Installer and lifecycle packaging is tracked by issue #16.

## Platform status

The implementation targets Windows APIs and the current probe binaries are built for Windows amd64. This is not a general support promise. Final supported Windows editions, versions, CPU architectures, privilege combinations, and limited-beta entry criteria remain owned by issue #18 and the physical Phase 0 gates.

Windows login and secure desktops, guaranteed control of elevated applications from a lower-integrity process, non-Windows systems, multi-monitor topology, and automatic updates are outside the MVP contract.

## Local data and diagnostics

Local credentials, trust records, launcher configuration, and reports stay on the device. UI and launcher code must display redacted summaries rather than raw credentials, peer addresses, input data, window identities, or unrestricted file paths.

Closing a GUI window must not abandon an active lease. Product shutdown must request agent cleanup, revoke control, release held input, and report whether the system reached `LOCAL`.

## Review blockers

A desktop change is blocked when it bypasses the agent ownership boundary, makes unsupported platform claims, weakens kit integrity or path validation, adds telemetry or content capture, or treats GUI termination as proof that input cleanup completed.
