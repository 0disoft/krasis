# Per-attempt grant progress

The optional `grantProgress` object on a source handoff summary describes the
process-local interval from that attempt's Begin to the matching grant handler.
It does not change the peer or protected-input wire protocol. Existing exchange
spans keep their definitions. This interval includes request coordination/write
and grant dispatch, so it is not just network time or the existing
RequestWrittenToGrantRead interval.

Schema 1 records three disjoint duration totals in `micros`: frame-reading,
frame-dispatch and between frames. `elapsedMicros` is the containing duration.
Nanoseconds accumulate before flooring to microseconds; the sum cannot exceed
elapsed and may differ by at most two microseconds. `complete` identifies a
matching grant handler; aborted, superseded or expired attempts retain partial
evidence with `complete:false`.

`byteProgressMeasured` means the TLS-decoded-byte callback was installed, not
that any byte arrived. `byteReads`, `bytes`, `frames` and `maxByteGapMicros`
describe progress in this interval. The maximum gap includes the start-to-first
and last-to-end intervals. A missing byte observer preserves absent evidence,
never a measured lack of network traffic. Counts saturate at uint32 maximum,
with `saturated:true`; they must not be interpreted as exact totals then.

Only the existing first twelve attempts within 120 seconds are profiled. Outside
an active grant wait, reader phase changes use a single atomic update and byte
callbacks skip the collector mutex. There is no timer, sampling goroutine,
per-byte event, payload logging or extra network request. The observation uses
real local monotonic times rather than the injectable policy clock.

Matching session/epoch association remains memory-only. Frozen, expired and
wrong-attempt callbacks cannot update a completed profile. The exported schema
accepts only bounded numeric fields; no timestamps, input, PIN, paths, addresses
or lease identities are added. Existing connection correlation labels can group
the exports; local attempt ordinals do not establish cross-host wire identity.

A read phase includes TLS/native read blocking, scheduling and peer response
time. Decoded-byte progress cannot prove when encrypted socket bytes arrived,
TCP retransmission, Wi-Fi loss or zero-window behavior. High local dispatch
duration identifies reader work that delayed further reads; a long read phase
with no decoded-byte callback narrows the observation but leaves its network
versus scheduling cause unresolved. Do not subtract durations across hosts or
add this profile to nested exchange/preparation spans.

Hosted validation covers phase conservation, zero/absent byte evidence,
matching and stale callbacks, partial retirement, frozen summaries, saturation,
window limits and sanitized export. Device behavior and observed overhead must
be checked on the integrated g174 bundle before any latency-improvement claim.

## g174 reader integration

Agent session startup installs the additional decoded-byte observer before
starting its reader and source goroutines, preserving the existing runtime
byte-progress callback. The transport, disconnect and supervisor wrappers
forward the optional capability and its actual availability. A fresh callback
captures each connection session; a retired connection cannot update the next
session's source attempt. Unsupported connections still record local reader
phases with `byteProgressMeasured:false`.

The peer reader records the phase transitions around ReadFrame and ordered
dispatch, reusing the existing runtime observer timestamps. The profile ends
at the matching source grant handler; later activation, revoke and input cannot
modify it. Connection failure retires a partial interval. No message order,
authority check, timeout, installation behavior or socket option changes.

Hosted session tests inject a 20ms read delay and a separate 20ms local policy
dispatch delay. Both preserve ordered grant/revoke cleanup and classify the
delay in the expected local phase. Fragmented frame tests verify both byte
observers see identical counts/timestamps and unchanged payload framing.
Wrapper, reconnect-scoping and sanitizer tests cover missing capability and
stale callbacks. These are functional timing-contract checks, not a benchmark.

Apply g174 on both devices, confirm protected PIN reentry still works, cross the
boundary several times and export fresh diagnostics. For a slow source attempt,
inspect `grantProgress` together with its existing exchange/preparation spans.
Do not identify separate machines' attempts by ordinal or claim packet loss
from a long read phase. Socket-level evidence remains a separate next step if
decoded-byte and local-dispatch evidence cannot explain the wait.

The [g174 export observations](g174-grant-progress-observations.md) confirm
that the first device trial produced complete numeric progress profiles. They
also identify a local receiver readiness outlier and an independent source
focus-restore delay; neither is evidence of TCP retransmission.
