# Cursor rollback before backend retirement (g177)

The g176 export has a receiver rollback failure at the `inspect` stage after a
source capture failure. In installed input, DisarmReturn closes the worker
lease that owns cursor reads and writes. Both immediate entry-failure cleanup
and prepared-lease abort previously called DisarmReturn before RollbackEntry.

The hosted regression models that installed lifetime: disarm retires cursor
access. Before the change, immediate entry failure, unknown grant-write failure,
peer abort and pending-session cleanup all fail recovery with an already-retired
backend. The corrected paths defer matching DisarmReturn until runtime release
and cursor rollback have finished. Retirement still runs once if rollback fails.

Pending state is detached under the existing session write coordinator before
prepared-lease abort. Exact session/lease/epoch checks still precede a peer abort;
a stale abort neither retires cursor access nor rolls back the valid entry.
Recovery retains its existing two-second context independent of request
cancellation, and original/cleanup errors remain joined. No cursor input is
retried, no backend fallback is added, and no worker authorization, heartbeat,
expiry, protocol, installer or security policy changes.

The change corrects receiver cleanup lifetime. A foreground-response probe
failure can still reject source activation, and an expired prepared candidate
can still require a fresh worker. Those separate failures/delays are not changed
or claimed resolved here.

Hosted validation covers the four cleanup paths, rollback success/failure,
cancelled entry context, bounded cleanup, unconditional retirement, preserved
error chains and stale abort rejection. Related agent tests continue to cover
pending-lease timeout cleanup and prevention of late commit. No native input or
privileged service is started by these regressions.

Apply g177 on both devices, repeat crossings and PIN-open B-A-B reentry, then
export fresh diagnostics. Confirm whether rollback `inspect` failure or reconnect
recurs; report source activation errors separately. Native behavior and perceived
latency remain device-verification work.
