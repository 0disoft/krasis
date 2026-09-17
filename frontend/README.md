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
requirement and installed agent generation are unchanged.

Double-click the executable while the existing Krasis controller remains in the
tray. It reads existing paired devices and supports refresh, connect and
disconnect. Closing this window does not stop or disconnect the agent.
Pairing, clipboard settings, updates, diagnostics and automatic recovery remain
owned by krasis-control.exe; this is not yet its replacement.

The host starts no input hook, agent or public HTTP listener. Local API
credentials stay in Go. The renderer receives only the narrowed bridge snapshot.
Repeated launches focus the existing Workspace window.

Configured `krasis_workspace_build` tests the frontend and compiles Windows
without CGO. `krasis_workspace_smoke` starts a hidden, read-only WebView for at
most 25 seconds, verifies renderer-to-Go-to-agent roundtrip, and exits. It never
connects, disconnects, restarts or updates the installed agent.

Local validation on 2026-09-17: seven frontend tests, native asset/Windows
compilation, productpanel race tests, and the hidden renderer/agent roundtrip
passed. Connect/disconnect were not triggered during the smoke check to preserve
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
Standard action icons remain Phosphor. The UI package is 0.2.0; this does not
change the installed Go agent generation or the Windows bundle.
