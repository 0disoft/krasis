# ADR 0057: Record the final runner failure

Generation 30 records the sanitized descriptor of a failed runner's returned error
before publishing session_failed. Previously finishSession ignored that error and
relied on supervisor snapshots, which do not carry a descriptor on every exit path.
The reported B log had DISCONNECTING and session_failed with no fault descriptor;
it cannot retrospectively establish which native or supervisor condition failed.

Supervisor exit wrappers identify source-close, connection-close, nonlocal-state,
unexpected-stop, and nil-connection branches. They preserve the original unwrap
chain, cancellation classification and errors.Is behavior. Final diagnostics record
both the underlying descriptor and the exit branch when present. A recovery deadline
has its own fault code. All codes pass through the existing closed-vocabulary export;
native error strings, paths, key content and window titles are never exported.

A per-session fault-code ledger prevents duplicate observer/final-return counters.
Stale runner completions and observers cannot add reasons to a later session.
Normal cancellation does not create a fault. Unknown causes explicitly use the
existing unknown descriptor rather than silently omitting a reason.

This repairs diagnostic coverage, not the unproven cause of the intermittent B
disconnect. Native recovery, ownership deadlines, LOCAL gating and the FAULTED
latch are unchanged. No API schema or database migration is required. Two-PC runtime
evidence remains necessary to identify the original intermittent trigger.
