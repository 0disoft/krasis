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
