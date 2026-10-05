# g181 entry mismatch classification

g180 retained a B entry/readback mismatch followed by reconnection. Its confinement
correction did not eliminate that failure. Existing exports cannot distinguish a
changed constraint, virtual-screen bounds, coordinate-space disagreement or
another cursor movement. This build classifies a mismatching placement before
choosing a correction; it does not claim to resolve disconnects or pointer delay.

## Local observations and limits

The tracked installed receiver asks for one probed placement in the existing
120-second/12-attempt collection window. Both the collector (across reconnect)
and each native worker port have a 64-request cap. Normal successful placement
adds no native queries; the receipt adds two comparison bytes to the same ACK.
Only mismatch performs additional read-only queries, at most one confinement,
one virtual-screen bounds snapshot and one physical cursor snapshot. Cancellation
stops subsequent queries. No cursor reset, replay, fallback or input policy change
is introduced.

The same `injection_failure` contains `cursorStage:mismatch` and optional
`cursorObservation:{known,flags}`. Bit values:

| Bit value | Comparison |
| --- | --- |
| 1 | Permitted target differs from original request |
| 2 | Actual readback matches permitted target |
| 4 | Confinement differs from its pre-placement snapshot |
| 8 | Virtual-screen origin/size differs from its pre-placement snapshot |
| 16 | Physical cursor snapshot matches permitted target |
| 32 | Physical cursor snapshot matches actual readback |

A comparison is known only if its bit is present in `known`. A known bit with an
unset flag is false. Missing observations are not all-false observations.
Readback-match is false for an exported mismatch. Failed native queries remain
unknown; physical comparisons share one snapshot and cannot both be true for
distinct target/readback points. Neither coordinates nor rectangle values are
logged. These snapshots are sequential; flags narrow alternatives without proving
which process, desktop change or input caused the mismatch.

## Validation and device follow-up

Focused tests cover window/role/cancellation gating, reentry and reconnect budgets,
concurrent native-port copies, one placement/scope, ACK alignment and truncation,
invalid metadata without altered placement outcome, query absence on success,
query failure/cancellation, stable/changed confinement and screen bounds, physical
comparison classes, rollback ownership, and local/peer sanitization. Native cursor
set/read/physical queries are stubbed; no PIN or live cursor input is injected.

Desktop Go tests and vet (`-unsafeptr=false`) passed for agent, transport, peer,
residentagent, handoff, inputworker, productlog, localapi, wininput and application.
Formatting and diff checks passed. Native device trials and a controlled latency
comparison are skipped here because they require the paired laptops and live
operator input.

Apply g181 to both devices, including the protected input workers. Use a fresh
app/agent run for the collection window. Cross normally, then open B's PIN dialog,
return to A while keeping it open, and reenter B several times. Export both
diagnostics promptly, especially after a blocked or disconnected entry. Trial
absence of mismatch does not establish a permanent fix. Native PIN trials and
measurement overhead/performance improvement require device evidence.

Source-of-truth change: ADR 0064, local placement probe/ACK and export contract.
No external API, database, runner, installer policy, ACL, UAC, certificate or
clipping-policy change. Procedure selection: feature, bugfix, desktop-app and
performance-measurement-integrity-review.
