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

## g212: Compact workspace and progressive settings

The default workspace combines peer selection, app-open, registration and
connection controls into one toolbar. Device illustrations show their names
and known relative placement; the fixed input-role captions are removed.
Clipboard sharing stays directly visible beside update, diagnostics and
settings actions. Duplicate registration, a static navigation tab, recurring
help and always-disabled placement controls no longer take default-page rows.

Settings contains arrangement/re-entry controls, keyboard lock/IME details,
startup/automatic-update preferences and language. Native generation remains
in Settings and the titlebar tooltip. Safety warnings, app-setting failures,
active public updates and active/failed clipboard transfers stay on the main
page. Unknown status is never turned into success to simplify the layout.
Consent, trust, connection operations and persistence remain unchanged.
Initial dialog focus selects a visible control or disclosure, skipping both
window controls and controls inside a collapsed settings section.

## g213: One-click diagnostic collection

DiagnosticsCollect accepts no renderer argument. The native bridge saves local
fallback evidence, resolves the active session alias against current non-revoked
trust, and starts the existing peer-bound diagnostic collection once. It retains
that request ID and observes it through DiagnosticsStatus. This reuses the CLI's
management API, authentication, sanitized exports and request-specific files.

The renderer exposes a single Save diagnostics action and an Open folder action.
Collection status describes fresh files for this request, separately from older
exports on disk. Complete requires the exact request ID, RECEIVED state and both
file paths in the native response; paths and IDs never cross into the WebView.
Pending, offline/local-only and partial results remain explicit. Lost start
responses are never retried by polling. Pending polls may resume after transient
errors but expire within a bounded wait. No new network endpoint, database or
runner is introduced; the existing legacy export binding remains available.

## g214: Trust validation uses one directory handle

Windows directory-security validation opens a read-only securefs handle with
OBJ_DONT_REPARSE and validates directory metadata and the canonical protected
DACL on that handle. It avoids the repeated per-component attribute walk,
Lstat and named ACL reopen. No trust verdict is cached and the one-second
monitor and five-second freshness deadline remain unchanged. Revocation and
missing/corrupt stores still stop the session. Terminal trust causes propagate
through cancellation to fixed fault codes instead of fault_session_unknown.

Diagnostic saving and status RPCs run on one native worker per bridge. The
desktop binding returns a copy of SAVING/PENDING immediately; polling and
repeat clicks cannot start overlapping workers or repeat START. The agent's
request-specific local export is the only export on the happy path. A local
fallback is written when START has not confirmed that snapshot. Legacy export
bindings remain synchronous. A blocked filesystem operation may outlive the
RPC budget, but cannot block renderer status or create unbounded workers.

The native diagnostic-collection-latest.json receipt persists request identity,
deadline and narrow status. An atomic SAVING intent precedes START to invalidate
an older collection; a crash before the returned ID is persisted restores an
unknown outcome and never retries START. Pending requests resume STATUS for the
same peer and ID. Restored saved indicators require their actual sanitized
snapshot files; missing, corrupt or unreadable records stay visible as failure.
Receipt errors do not turn collection success into durable success, and an
unwritable intent prevents remote collection while retaining a local fallback.
IDs stay outside the WebView. Open folder selects the validated local snapshot
of the current collection in Explorer, with folder access retained when the
receipt cannot be validated. No new network endpoint or database is introduced.

Hidden-window smoke reports rendererAgentRoundtrip separately from agentHealthy,
agentState and agentCode. A real FAULTED/recovery_required response proves the
bridge roundtrip but fails the overall smoke with exit 2. Unknown or malformed
responses never count as a roundtrip. Smoke does not recover or replace agents.

## g215: Fit startup windows to the monitor work area

The normal workspace starts at up to 1000 by 640 DIP. It stays hidden until
runtime-ready, then fits and centers in the window's actual monitor WorkArea
before showing. Both minimum dimensions shrink on small or scaled displays;
the taskbar and monitor offsets are respected. Wails provides DIP coordinates,
so they are not scaled a second time. Tray/second-instance reopening clamps an
off-screen normal window while preserving an already fitting user placement.
Maximized/fullscreen windows retain OS sizing. No periodic resize or input,
connection, API, database or preference-persistence change is introduced.

## g216: Tray menu follows the selected UI language

The existing workspace locale preference remains the source of truth. The
renderer sends its resolved ko/en locale to SetTrayLocale at mount and after
language changes. Native code owns all four menu labels, validates the locale,
and updates the existing items on the UI thread without changing callbacks.
Updates serialize so an older response cannot overwrite a newer selection.
Only one language appears in each menu item. Failed application stays visible
in language settings. No new preference file, network API, database, input or
connection behavior is introduced.

## g217: Keep device updates focused on the update

The update dialog shows the two generation numbers, comparison, send action
and transfer progress. Normal protected-input status is no longer repeated
here; required input setup or recovery stays on the main workspace screen.
Build identifiers and receive-permission controls remain available under
collapsed Details. Denied or unknown peer permission stays visible beside
the send action, so folding details never hides a reason for a blocked update.
Unknown peer state does not claim offline, and equal generation numbers with
different builds remain a conflict rather than claiming the latest version.
Confirmation, permissions, polling and transfer verification are unchanged.
No network API, database, repository hygiene or runner changes are introduced.

## g218: Refresh settings and bound utility actions

The settings dialog reports its open state to AppSettings. Each reopening
reads the actual native settings again, replacing stale failure indicators
only with the returned status. Initial background loading still supplies
workspace notices. Reads do not overlap an active settings write and closing
the dialog does not cancel or retry a write. No preference defaults change.

Peer update sending is enabled only for a freshly observed LOCAL_NEWER
comparison in addition to the existing permission and generation checks.
Matching builds show a disabled Up to date action. Newer peers, conflicting
builds and unknown comparisons remain explained and cannot initiate a send.
Native version, trust and permission checks remain authoritative.

Keyboard observations run only while the settings dialog and keyboard details
are both open. Hiding either cancels the next poll and rejects a late result.
Reopening or changing peers clears old observations before a fresh read.
In-flight reads remain serialized with matching, and cannot restart a hidden
poll loop or enable matching with another peer's observations.

Manual release checks immediately return CHECKING once the existing native
worker accepts the request. Pending and active checks coalesce, while a
staged READY release remains stored for recovery if checking fails. Status
projects pending work until the worker starts. The renderer disables repeat
clicks immediately and discards pre-click polling responses using an epoch.
No release schema, new endpoint, signing policy or download policy changes.

The pairing dialog explains that a connecting, connected or reconnecting
session must be disconnected before registering another device. Opening
the dialog itself does not change the connection or start a pairing job.
No API schema, database, repository hygiene or runner changes are introduced.

## g219: Bound peer launch receipts and recover settings failures

Expired or invalid peer-launch storage cannot block a new explicit request.
Only the initial acknowledged request creates a deadline; polling never
renews it. Repeated opening resumes a valid saved ID without sending START
again, and a terminal response clears only its matching saved receipt.

Startup change failures inspect and compensate every affected OS setting,
including a partially changed recovery task. Both writes and subsequent
reads must succeed before restoration is verified. Unknown reads never
authorize blind rewrites. Fixed settings_change/settings_restore events
retain the failed stage and compensation outcome without raw error text.
The additive rollback_failed settings code stays explicit in the renderer.

Disabling automatic updates marks cancellation of pending or active checks.
Only a marked context cancellation becomes CANCELLED with a fixed diagnostic
event; timeout, validation and download failures remain failures. A pending
cancel is honored before network work starts. Downloaded files are retained,
and cancellation never applies them. Explicit new checks reset the marker.

Peer-launch controls are keyed by the selected trusted fingerprint. Switching
peers disposes the old visible polling loop without canceling its native
request. Returning resumes the saved ID with its original deadline. Client
gates are per peer, so an outstanding query cannot lock another peer's action.
Late responses stay with their peer and never replace the current control.
Native trust and request gates remain unchanged. Settings/release codes gain
rollback_failed/CANCELLED; no new endpoint, database, runner or hygiene changes.

## g220: Keep utility state honest across clock and settings changes

Peer-launch observation windows use elapsed time in the current renderer,
shared across peer-control remounts. A reloaded stored ID is confirmed by a
native status query before it receives one bounded observation window.
Neither wall-clock correction nor local timeout discards the ID or sends
START. An explicit replacement first requires native expired/unknown_request
confirmation. Further pending polls never renew an established window.

The running service retains unverified settings restoration separately from
generic registration failure. Subsequent reads keep rollback_failed while
settings, startup registration or the recovery task remain unreadable or
mismatched. Only a fresh read confirming both OS states against the saved
preferences clears that marker, with a fixed settings_restore_all_verified
event. This reconciliation never writes registry or task settings.

READY describes a retained, verified download, not permission to auto-apply.
Its renderer notice follows the confirmed automatic update preference: enabled
waits for disconnection, disabled explains enabling the setting to apply, and
an unreadable or pending setting only asks for verification. Native application
still requires successful settings reads both on the tick and under the
settings gate immediately before application. No new manual-apply endpoint,
release schema, database, repository hygiene or runner changes are introduced.

## g221: Preserve update reenable requests and unknown settings

An explicit check or reenable while a release check is being canceled retains
one follow-up request. It is queued only after the canceled worker completes,
using a fresh context. Further requests coalesce. Disabling again or shutting
down discards the deferred request. An old canceled context is never revived.

The additive preferencesKnown field distinguishes readable saved preferences
from OS registration/restoration outcomes. Unreadable values stay unknown even
with rollback_failed. Checkboxes are indeterminate and disabled until a fresh
read succeeds. Failed-write reconciliation never reuses stale known values if
its subsequent read fails. Legacy error responses cannot prove preferences.

Both the workspace banner and settings notice use appReleaseNotice with the
same preference snapshot. A pending settings read or write projects unknown
preferences to both surfaces, so neither announces automatic application from
stale values. Duplicate release strings are removed from the native catalog.
No endpoint, database, repository hygiene or runner changes are introduced.

## g222: Retain downloads, separate loading, and expose UI replacement

Release checking state is independent of the staged generation. The additive
readyGeneration field retains a verified download across cancellation and
network failures. Each new check restores and verifies the disk receipt before
network work; application revalidates the package and current settings. Failed
handoffs suppress repeated application until another check verifies the cache.

Settings checking is renderer-local pending state, distinct from unavailable.
Neither startup nor subsequent pending reads render an error banner from an
unresolved response. Once the read completes, genuine failures remain visible.

UIReplacementStatus exposes bounded state/generation/canRetry only. Preparing,
pending, applying and failed replacement requests appear in the workspace.
RetryUIReplacement takes no arguments, retains the last explicitly opened local
source, revalidates the whole bundle, and does not automatically replay failed
launches. Busy handoffs keep the existing bounded polling cadence. Renderer
epochs reject reads predating a retry; uncertain acknowledgements are queried,
never automatically retried. No database, hygiene or runner changes are made.
An explicit second-instance launch also brings the current management window
forward, so pending or failed replacement is visible even from tray-only state.
