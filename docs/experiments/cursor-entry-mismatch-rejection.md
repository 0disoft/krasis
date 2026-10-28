# Reject a recovered cursor entry mismatch before granting input

The paired g474 collection `bff0e1ad36495e5d710a6d77d096cf66` contains a B
`entry/mismatch` at `2026-10-28T08:49:36.5811787Z`, followed by
`fault_control_request_entry_failed` and reconnect. Both services remained READY;
this was a session failure, not recovery error 1066. The failure has no placement
observation, so the export cannot distinguish physical movement, changed cursor
constraints or a coordinate-space issue. None is established as its native cause.

Previously every PrepareEntry error terminated the connection, even if the
adapter had obtained the pre-write snapshot and readback and then completed its
conditional rollback. At this point no grant, inbound stream or source capture
activation exists. Destroying the authenticated connection is unnecessary for
this narrow, recovered preparation conflict.

The adapter now marks only a readback/target mismatch with the local
`application.ErrCursorEntryMismatch` sentinel. Stage text alone is insufficient.
The responder rolls back using the existing cancellation-independent cleanup
budget and retires the exact return/input owner after rollback. Only a marked
mismatch with no cleanup error, a RESTORED or NOT_RESTORED outcome and a live
request may send the existing exact-session/epoch LeaseReject. NOT_RESTORED
preserves a cursor that moved after observation instead of restoring stale state.

The source remains unsuppressed, returns its pending request to LOCAL, and the
same connection can authorize a fresh request. A rejected request is never
committed or replayed. Placement ACK loss, read failure, rollback error or unknown
outcome, cancellation, and failed rejection writes remain terminal. Native
placement still performs one cursor write; no placement retry, fallback, clip
change or input-certainty downgrade is added. The existing failure log remains.

Regression tests cover both known rollback outcomes, rollback-before-retirement,
the source's rejection and subsequent receiver grant, and six terminal failure
cases. Native adapter tests verify the marker and retained rollback ownership.
This fixes the unnecessary connection teardown; it does not establish why B's
cursor moved or resolve A's capture latency. Paired device verification requires
installing this revision on both PCs and collecting fresh round-trip diagnostics.

There are no wire format, authentication, database, installer, dependency or CI
runner changes. Reverting this change restores terminal handling for mismatches.
