# Krasis control UI — preview and native Workspace

Svelte 5 desktop UI based on the selected Midnight Workspace reference.
The default browser build is an isolated preview, not a replacement for
krasis-control.exe. Its device state is in memory; only language is stored.
No agent endpoint, bearer token, clipboard API, PIN, process or network control is
exposed to the renderer. Pairing actions are explicitly unavailable in this stage.

Korean and English catalogs cover navigation, accessible labels, dialog copy,
state messages and settings. Explicit locale wins over navigator.languages;
unsupported languages fall back to English. No remote font requests are made.

The default browser build remains simulated. The separate native-mode build uses
NativeApp.svelte with authoritative agent state and real connect/disconnect.
Do not package the browser preview as the working KVM management app.

`src/bridge.js` validates the native-owned `productpanel.DesktopBridge`
snapshot contract. NativeApp.svelte consumes it; simulated App.svelte does not.
See ADR 0061 for its identity, concurrency and cancellation boundaries.

## Native Windows Workspace

The optional Wails v3 host builds to `dist/workspace/krasis-workspace.exe`.
It embeds the native Svelte assets and uses the Go module's own
`/wails/runtime.js`; there is no separate npm runtime version to synchronize.
Wails v3 is an explicitly selected prerelease dependency. The Go language
requirement is unchanged; the bundle includes agent generation 36, and building
does not replace the running process.

Extract the complete Workspace ZIP and double-click krasis-workspace.exe.
The sibling agent is started automatically after bundle checksum verification.
The existing controller is not required for startup, connection or recovery.
Closing the window keeps Workspace in the tray; its menu can reopen it,
disconnect, or quit only the controller while leaving the agent running.
PIN pairing and left/right placement are available in Workspace. Update SEND is
explicit from Workspace to the selected trusted peer; no legacy control is
required on the sender. The g35 bundle keeps the g34-compatible
krasis-control.exe filename while the receiving UI becomes Workspace after
validation. The first g34 to g35 update needs the existing control running on
the receiver; afterwards a running Workspace suffices. The Workspace receiver
auto-applies previously permitted updates through the native worker, keeping
stored trust and permissions and existing directories. A received bundle sends
from its own root when its binary is krasis-control.exe, otherwise from the
sibling peer-update folder.

## Remote update send and status

Workspace 0.9.0 adds explicit SEND from Workspace to the selected trusted peer,
plus exact status for that peer fingerprint, transfer ID, and local agent
instance. Sending is manual
only; there is no automatic resend. The bundled `peer-update` folder contains the
g35 Workspace agent/control bundle under the g34-compatible filenames. The first
g34 to g35 update needs the existing control running on the receiver so it can
automatically APPLY the received bundle; afterwards a running Workspace suffices.
The receiver switches to the Workspace UI automatically after validation, and
sending updates the recipient to Workspace. Extract the entire Workspace ZIP before running.
Peer management-channel observations are online only on a
response within 15 seconds, stale for older observations, and unknown when there
are no observations. Version comparison uses generations and build identities,
not semver; different build identities in the same generation cannot be ordered.
Older local agents without the per-peer INSPECT action report peer status as
unavailable while existing permission settings remain usable. Automated tests
do not mutate real permissions. Actual two-PC update delivery has not been tested
on real devices.

The per-peer INSPECT local API was added in agent generation 33, included with
the Workspace UI 0.8.0 bundle (agent generation 34) and carried in the 0.9.0
bundle (agent generation 35). Agents below generation 33
report peer-info lookup as unavailable while the existing permission API continues
to work.

## Manual diagnostics

Workspace 0.6.0 adds manual diagnostics: save locally, save and send to the
selected trusted peer, and open the diagnostics folder. The panel shows local
and received availability separately, and the transfer code is the latest global
diagnostic result rather than the selected device. No paths or diagnostic
contents are exposed to the UI. Two-PC diagnostic delivery remains unverified
on real devices.

## Clipboard sharing

Workspace supports the existing opt-in text sharing protocol. Enable it on both
PCs using the clipboard card. Enabling requires a warning/confirmation; disabling
does not. The switch reflects acknowledged agent settings, not optimistic UI.
Only the peer fingerprint and a status code cross the WebView boundary. Copied
text stays in the native Windows worker and the authenticated local agent.

The worker reuses the legacy controller's Unicode size limits, excluded-content
formats, remote-write marker, exact sequence comparison and acknowledgment logic.
It checks active trust and sharing consent before clipboard access. A process-
owned UI window receives Windows clipboard-owner notifications; the worker
does not create an unpumped owner window. Closing to tray keeps the worker alive.

If the legacy controller is also open, Workspace defers to its existing pump.
On takeover, changing peer, revocation or disabling, the baseline resets so old
clipboard contents are not republished. Files/images are not supported.
Consent changes and native synchronization are serialized. An off/on cycle,
including an ambiguous setting response, resets the native sequence baseline
before synchronization resumes.
The read-only smoke mode does not start a clipboard worker or change consent.

The host owns no input hook or public HTTP listener. Local API
credentials stay in Go. The renderer receives only the narrowed bridge snapshot.
Repeated launches focus the existing Workspace window. Startup and replacement
reuse productpanel.Startup, its native startup lock and bundle verification.
Automatic recovery runs in Go, independent of hidden WebView timers. It saves
diagnostics before replacement, attempts bounded peer delivery, and retries at
most three times using the original peer fingerprint. Failed local export blocks
replacement. Disconnect cancels an in-flight recovery and stops further retries.
RECONNECTING remains owned by the existing agent supervisor, not a restart loop.

Configured `krasis_workspace_build` tests the frontend and compiles Windows
without CGO. `krasis_workspace_lifecycle_build` additionally builds the sibling
agent and packages checksums and notices. `krasis_workspace_smoke` starts a hidden, read-only WebView for at
most 25 seconds, verifies renderer-to-Go-to-agent roundtrip, and exits. It never
connects, disconnects, restarts or updates the installed agent.

Local validation on 2026-09-17: frontend tests, native asset/Windows
compilation, productpanel race tests, and the hidden renderer/agent roundtrip
passed. Lifecycle regressions cover the retry budget, failed exports, cancellation,
changed peer identity and worker shutdown. The smoke mode intentionally bypasses
automatic startup/recovery and tray installation. Connect/disconnect were not triggered during the smoke check to preserve
the active user session. WebView2 emitted a window-class unregister warning
(1412) on shutdown; the process exited successfully. Visible native layout and
full legacy-controller parity are not covered by this smoke result.

Configured verification: `krasis_frontend_install`, `krasis_frontend_check`.
Configured preview: `krasis_frontend_preview` serves only loopback port 4179 and
stops after ten minutes, or when its development-only `/__preview_stop` endpoint
is opened. It does not persist as a background service. See design-qa.md for the
visual comparison evidence and remaining native integration work.
Dependency install is scoped to frontend and disables lifecycle scripts.
The selected major lines follow stable updates; package-lock records the build.

Monitor artwork: `src/assets/monitor.png`, generated with the built-in image tool
from the selected Midnight Workspace image. Prompt: recreate one front-facing
neutral charcoal monitor with a fine grey outline and stand, no text, cursor or
background dots. Device labels remain live localized UI, not baked into the asset.
Clipboard integration adds focused `krasis_workspace_clipboard_check` coverage
for the shared pump, controller-presence probe and native controller regressions.
Automated checks do not read or replace the user's current clipboard; two-PC
copy/paste and visible consent-dialog testing remain manual.

Standard action icons remain Phosphor. The UI package is 0.9.1 and the Workspace
bundle includes agent generation 36. Building the bundle does not replace the
running agent. UI 0.9.1 is a responsive/scrolling modal and numeric rearm-selection
fix; existing g35 receivers can accept this revised UI (same-generation
different-build stays intentionally blocked).
