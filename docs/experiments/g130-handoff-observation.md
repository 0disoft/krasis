# g130 handoff observation

Build: generation 130, revision `a36d43daa4325e67ea176e82a9a38fbfd04ad8aa`.
Source: paired sanitized support exports received on 2026-10-03 (KST), and
read-only CLI status confirming both peers at this revision and connected.
Input content, private identifiers, addresses and absolute paths are omitted.

## Evidence and exclusions

Each agent exported 12 profiles. A has source profiles only; B has receiver
profiles only, all selecting the native input backend. There are no installed
worker connect/ACK measurements and no B-source handoff profiles. Absence does
not prove the user never attempted those actions: the budget was already used.
The old successful installation receipt on B is not a live backend proof.

A's first completed profile preceded B's g130 update. Exclude it from the
both-g130 range. Also exclude unfinished profiles and any frame vectors from
the last slot: abort retirement and budget exhaustion permitted later leases
to append to that slot. Regressions reproduce both errors against g130.

Remaining data: A attempts 2-11 and B attempts 1-10, ten completed profiles
on each host. These are host-local software measurements, not cross-host
end-to-end or display presentation timings. No wall clocks are subtracted.

| Host-local stage | Observed range |
| --- | --- |
| A request begin to grant received | 15.843-24.095 ms |
| A source capture activation | 5.480-14.837 ms |
| A first-frame send processing | 0-7.136 ms |
| B authorization wrapper (native input) | 6.708-18.360 ms |
| B first-frame receive processing | 0-11.283 ms |

The zero entries are measured values at timer/microsecond resolution, not a
claim of zero work. Stages overlap and must not be summed. Steady-state frame
histograms exclude the first frame: A has 781 measured sends (778 <1ms, 3
<10ms), while B has 774 measured dispatches (596 <1ms, 175 <10ms, 3 <100ms).
Counts are local measured frames, not physical inputs or proof of paired
frame identity. The longest observed steady-state dispatch among these
completed records is 1.512ms on A and 12.767ms on B.

## Failures and limits

A logged seven `activation_capture_foreground_unavailable` failures. Those
identify source capture foreground acquisition failures, not slow worker
startup. A later logged `protected_input_detected` and
`protected_input_reconnecting`; B logged remote EOF and reconnecting. The
software evidence identifies A's conservative protection path as the local
disconnect trigger. It does not identify the actual foreground UI or prove
which PIN/screenshot interaction, if any, occurred.

No 0.5-second software delay appears in the completed records. This does not
rule out perceived display lag, unprofiled attempts, failed capture pauses or
installed-worker latency. The contaminated last profiles cannot establish a
15-second startup delay or steady-state slowdown.

## Follow-up

g131 retires failed source activation and exact inbound abort profiles at the
existing runtime recovery boundary, preserves the original runtime return
value, and detaches stale role association when a new attempt exceeds the
budget. Tests cover both source/receiver exhaustion, recovered capture abort,
and stale versus matching inbound abort.

Repeat the bounded trial from `handoff-profile.md`, using only three entries
per physical source so both directions fit the budget. Confirm the selected
receiver backend before comparing to the earlier installed-input reports.
The source foreground acquisition failures remain a separate unresolved
product issue; g131 corrects diagnostic association, not that failure.
