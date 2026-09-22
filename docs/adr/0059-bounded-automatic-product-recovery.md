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

General support export is best effort: observe at most six seconds, run one
concurrent export worker, and never let a late result change lifecycle state.
A save failure or delay keeps the diagnostic_failed guidance but does not block
Startup.Ensure or a normal reacquire. State saves and guards required for input
safety still block. Queue transfer again after agent replacement (the old queue
was in memory). Offline remote delivery is best effort and not reported as
successful. Own and received diagnostic filenames remain separate. No API or
persisted trust format changes. Both PCs need the new management app running,
including in the tray.

The original automatic recovery policy shipped in generation 32 with ADR 0058.
The current best-effort diagnostic policy revises only the general
support-export observation described above; the three-attempt budget, exact
alias/fingerprint revalidation, user-disconnect cancellation, and old-process
ownership release/LOCAL/guard requirements are unchanged. Tests cover retry
caps, brief reconnects, backoff, unknown peers, manual stop, and
reconnect-only exports. Temporary-file IO is not forcibly cancelled; a late
export result is ignored for lifecycle decisions while completed file handling
follows the existing path. Native two-PC crash/recovery testing remains a user
hardware acceptance check.
