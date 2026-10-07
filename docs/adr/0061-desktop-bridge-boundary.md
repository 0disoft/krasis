# ADR 0061: Native-owned desktop bridge contract

- Status: Accepted as integration foundation; not production cutover

`productpanel.DesktopBridge` exposes Refresh, Connect(alias, fingerprint) and
Disconnect. The native window supplies its lifetime context and a fresh local API
credential opener. The renderer never supplies a directory, URL, bearer token,
command line or process identifier. This bridge starts no server or background
worker and adds no Wails/WebView dependency yet.

Calls reuse productpanel.Execute and agent authorization. Connect rechecks the
selected fingerprint immediately before forwarding the alias; the existing agent
remains responsible for authoritative trust enforcement. A busy bridge rejects
overlapping work instead of queuing potentially stale user actions. Host shutdown
cancels calls, with a ten-second operation budget. Failed/unknown state is not
replaced by LOCAL or CONNECTED. Disconnect does not require a healthy trust store.

The frontend adapter in src/bridge.js accepts a native-owned binding and validates
the versioned snapshot. It strips unexpected fields, redacts rejected native
errors, and never falls back to simulation. This adapter is not yet wired into
App.svelte: the browser preview remains explicitly simulated.

Before native cutover: bind the three methods in a desktop host; replace simulated
state with serialized refresh/actions; map legacy panel Notice text to locale
codes; retain tray/single-instance/update/recovery ownership; test two-PC behavior.
FAULTED recovery is intentionally not implemented by this bridge and must remain
with the native lifecycle controller. Never translate Disconnect into an implicit
process kill. No new network API or database contract is introduced.

Validation uses deterministic mocked clients and adapter tests. It does not prove
native host binding, real input transfer or production window parity. Agent
generation and existing Windows packages remain unchanged until integration.

## g211: App-owned window titlebar

The workspace uses a frameless Wails window with a fixed Krasis titlebar. It
shows the app brand and native generation, and uses the existing Window runtime
for minimise, maximise/restore and close. Native drag regions retain Windows
window movement and edge resizing; controls opt out of dragging. Maximise/focus
events update chrome state without reloading or adding a polling loop.

Modal dialogs render the same shared titlebar in their top layer so native
window controls remain usable while the rest of the workspace is inert. Initial
dialog focus skips window controls. Closing retains the existing native
WindowClosing hook: hide to tray, keep the agent/session alive. WindowVersion
exposes display-only generation metadata, never an executable path or command.
Input, trust, transport and privileged installation contracts are unchanged.
