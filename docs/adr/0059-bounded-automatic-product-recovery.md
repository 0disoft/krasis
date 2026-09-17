# ADR 0059: Bounded automatic desktop recovery

- Status: Accepted

After the running management app observes CONNECTED, remember the actual agent
PeerAlias and its trusted fingerprint, not the selected dropdown entry. On
FAULTED or RECONNECTING export local diagnostics and queue transfer through the
existing paired management channel. A and B run this independently; input
connection loss does not prevent the management channel from being used.

For FAULTED only, allow three replacement attempts per user-started connection,
with bounded backoff. Brief successful connections do not replenish the budget.
Use Startup.Ensure: no force kill, no clearing the fault latch, no replacement
until the old process releases ownership, and no connection until LOCAL and the
same alias/fingerprint are revalidated. The existing supervisor owns ordinary
RECONNECTING retries. Manual disconnect disables automatic recovery; explicit
connect begins a new budget. Do not infer a target from a pre-existing fault on
GUI startup. Updating or pairing takes precedence.

Before replacement, local export must succeed. Give queued transfer five seconds
to complete and queue again after agent replacement (the old queue was in memory).
Offline remote delivery is best effort and not reported as successful. Own and
received diagnostic filenames remain separate. No API or persisted trust format
changes. Both PCs need the new management app running, including in the tray.

Agent generation 32 bundles this GUI policy with ADR 0058. Tests cover retry caps,
brief reconnects, backoff, unknown peers, manual stop, and reconnect-only exports.
Native two-PC crash/recovery testing remains a user hardware acceptance check.
