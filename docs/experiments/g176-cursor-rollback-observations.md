# g176 cursor rollback observations

Revision: `e367c8578677fe5a88d3c6a8bec423ea60b3ee18`.
Operator diagnostic exports, not a controlled performance comparison.

| Export | Created UTC | SHA-256 |
| --- | --- | --- |
| A | 2026-10-05 14:16:28Z | `17B4BAF3FE79CDF65585A1CB2906592E24A7557DE40B6D03AF62316F788EBDFD` |
| B | 2026-10-05 14:16:23Z | `93F3B59A3CA7EA3F50032D95E21B06FA23D324194DBBD99B5C20632DE3207B56` |

## Connection and failure evidence

Both hosts contain twelve profiles: eleven complete `ok` and one incomplete
`error`. Event run/sequence identities are unique, and histogram sums match
frame counts: A 1,169 [1153,16,0,0,0]; B 1,161 [92,1057,12,0,0]. Neither export
contains current-revision events in an incomplete historical log. Per-host
attempt ordinals are not a cross-host correlation key.

A records `activation_capture_foreground_probe_failed` at 14:15:53.1720378Z.
Its failed source profile has a complete 32.911ms grant interval with two
decoded reads, 116 bytes and one frame, followed by a failed CaptureActivate
lasting 137.230ms. Thus this source failure occurs after a received grant;
it is not a missing grant or confirmed receiver entry failure.

B records no `entry` injection failure. It records one `rollback` injection
failure with `cursorStage:inspect` at 14:15:53.2626955Z, alongside
`installed_input_cursor_read_failed`. Counts remain unknown, as does the
receipt's injection outcome. Its failed receiver profile has a successfully
written grant, EntryPrepare 4.538ms, decision 1 (`used`), prepared age 363.178ms
and ReadyWait 7.621ms. The stage identifies the current-position read during
rollback; it does not identify a native Win32 cause.

B then records `reconnect_injection_not_applied`; A records `reconnect_remote_eof`.
Both later record CONNECTED. No failures or reconnects follow the last CONNECTED
before export: about 32.4 seconds on A and 27.3 seconds on B. B's initial
network retries and disconnect request precede the trial connection. A PIN
trigger or manual intervention was not separately reported for this export.
Cross-host timestamps above are chronology labels, not synchronized elapsed
measurements or proof of a shared attempt.

## Static rollback-order finding

In `internal/peer/sender.go`, both the entry-failure cleanup closure and
`abortPendingLocked` call Returns.DisarmReturn before Entry.RollbackEntry.
In the installed path, `internal/residentagent/privileged_input.go` implements
privilegedBoundary.DisarmReturn by disarming local return tracking and then
closing the matching worker lease. Therefore these cleanup paths retire the
cursor backend before attempting cursor rollback. This source ordering is
consistent with the observed rollback `inspect` failure; no live/native
reproduction or corrected-order trial has yet been performed.

Existing responder tests use a recording cursor entry whose DisarmReturn does
not retire its cursor backend. They do not model the installed-path lifetime.
The next bounded fix should add a regression that retires cursor access on
disarm, then complete rollback before retirement on both cleanup paths. Preserve
cancel-independent bounded recovery, runtime release, exact lease matching,
failure aggregation and unconditional retirement even if rollback fails. Do
not retry uncertain input or remove the source foreground-response check.

## Independent delay evidence

B has eleven prepared-worker decisions and one decision 3 (`expired`). Ten
successful prepared profiles wait 7.007..8.589ms for readiness. The successful
expired-candidate profile launches a fresh worker: ReadyWait 117.149ms,
ConnectTotal 118.836ms, AuthorizeTotal 128.831ms and request-read-to-grant-written
132.995ms. These durations nest and must not be summed. This now identifies
expiry as the cold-start decision in this cohort; expiry should not be extended
without evidence about preparation timing and worker lifetime.

Successful-profile A RequestToGrant min/median/max: 28.672/32.940/145.895ms;
FirstOfferSinceBegin: 33.767/40.255/358.524ms; SourceDeactivate:
14.367/24.034/168.793ms. The largest SourceDeactivate contains a 160.724ms
focus-restore span. B request-read-to-grant-written: 21.635/24.635/132.995ms;
EntryPrepare: 4.131/4.549/5.736ms. Each of A's twelve grant receipts is complete,
byte-observed and unsaturated, with its entire interval assigned to reading and
zero measured reader dispatch. This still leaves peer response and scheduling
inside the reading duration; it does not prove TCP loss or a latency improvement.

## Validation and scope

Exact-revision isolation, event identity uniqueness, histogram conservation,
selection invariants, grant duration conservation and byte-gap bounds pass.
The g176 rollback stage survives the peer-export boundary. `git diff --check`
passes. Tests, rebuild, native reproduction and controlled comparison are
skipped for this evidence-only update. The rollback fix remains pending.
No API, protocol, database, installer, runner or security-policy change.
