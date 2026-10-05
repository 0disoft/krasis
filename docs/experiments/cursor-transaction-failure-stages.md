# Cursor transaction failure stages (g176)

The g175 exports contain an initial entry failure and a later rollback cursor
read failure. The later read event does not identify the initial fault.
g176 adds optional `cursorStage` to each existing `injection_failure` receipt
for `entry` or `rollback`, preserving their separate records and chronology.

| Operation | Stage | Boundary that failed |
| --- | --- | --- |
| entry | context | Missing or already cancelled operation context |
| entry | state | Adapter unavailable or another entry pending |
| entry | snapshot | Read the pre-entry cursor position |
| entry | geometry | Calculate a valid entry target |
| entry | set | Write the entry cursor position |
| entry | verify | Read back the entry cursor position |
| entry | mismatch | Read-back position differs from requested position |
| rollback | state | Transaction state unavailable |
| rollback | context | Missing or already cancelled recovery context |
| rollback | unverified | Pending write has no verified entry position |
| rollback | inspect | Read current position before deciding whether to restore |
| rollback | restore | Write the previous position |
| rollback | verify | Read back the restored position |
| rollback | mismatch | Restored position differs from previous position |

Only these fixed stage strings are exported. They do not identify the native
cause: a mismatch does not prove pointer clipping; a read failure does not
prove a desktop switch. Cancellation during a call is attributed to that call's
stage, not retroactively to the initial context check. No coordinates, targets,
PIN, input content or error strings enter the receipt. Position-mismatch error
messages no longer contain coordinates.

The typed local wrapper preserves errors.Is/errors.As and injection certainty.
Success creates no wrapper or new event. Existing transaction state, cleanup,
pending-write retention, cancellation and unknown-outcome behavior remain
unchanged. No input is replayed and no fallback backend is introduced. Existing
insertion-count validation remains: cursor calls with no valid insertion counts
retain unknown counts rather than inventing SendInput evidence.

Local and peer sanitizers both validate stage against operation. Missing stage
remains valid for older exports. Invalid or cross-operation stage in an imported
receipt is rejected; an invalid local annotation is omitted. No worker/peer ACK,
protocol, service authority, installer, security policy or runner changes.

Hosted checks inject faults at each entry and rollback stage without native
cursor I/O, verify distinct stages and retained causes, and exercise both
sanitizers. After applying g176 on both devices, repeat ordinary crossings and
PIN-open B-A-B reentry, then export diagnostics. The next fix should use the
first entry stage and subsequent rollback stage independently. This addition
provides failure evidence; it makes no latency-improvement claim.
