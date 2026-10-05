# g173 PIN-open reentry observations

Revision: `ecb3d5afd67a231a8aba11e1269aa7ae7b2cef6a`.
Both devices were requested to apply g173 and leave B's PIN dialog open while
returning to A and entering B again. The operator replied "완료. 잘 되네".
The report confirms the requested behavior; exact repetitions and timing were
not reported. PIN values, window text and pointer coordinates were not collected.

## Export evidence

Use only events matching the revision above. A's export was created at
2026-10-05 12:53:53Z and B's at 12:53:47Z. Connected-to-export observation is
about 50 seconds on A and 44 seconds on B, using each host's own timestamps.
Neither export has post-connect failure, reconnect or session-stop events in
that interval. B's initial RECONNECTING precedes CONNECTED. The matching
revision has no events in incomplete historical logs.

Each device contains twelve complete profiles ending `ok`; this is not
twenty-four independent transfers. The collector's twelve-attempt/120-second
limit prevents a claim about every subsequent transition or long-term stability.
Profile frame/histogram sums match: A has 389 frames, buckets [376,13,0,0,0];
B has 342 frames, buckets [27,310,5,0,0]. Event identities are unique within
each export. B has 22 installed-input lease-ready events and all twelve receiver
profiles report a prepared worker.

## Remaining transition-delay evidence

All values below are measured process-local spans over twelve profiles, in ms.
They are observations from this trial, not a controlled version comparison.

| Host and span | Min | Median | Max |
| --- | ---: | ---: | ---: |
| A RequestToGrant | 28.662 | 35.520 | 626.420 |
| A FirstOfferSinceBegin | 33.633 | 51.456 | 634.812 |
| A CaptureActivate | 5.516 | 9.817 | 26.296 |
| A SourceDeactivate | 7.659 | 24.367 | 91.409 |
| B EntryPrepare | 4.108 | 4.8985 | 5.918 |
| B ReadyWait | 6.610 | 7.535 | 8.588 |
| B per-profile worst Dispatch | 1.907 | 5.418 | 17.317 |

A's sixth source profile records RequestWrittenToGrantRead 626.420ms and
GrantReadToHandler measured zero; its FirstOfferSinceBegin is 634.812ms.
B's twelve RequestReadToGrantWritten values range from 22.330 to 35.488ms.
These independent host durations do not identify a matching wire attempt by
the local attempt ordinal. Do not subtract them to infer one-way network time.
Nested spans and per-stage maxima must not be added.

## Next bounded task

Prioritize the transition request/grant delay under GitHub issue #49. Capture
bounded, same-attempt local evidence for transport progress and scheduling
while A waits for the grant, reusing the existing TLS diagnostic correlation
and byte-progress observer. Preserve control/input order, authorization,
deadlines and fail-closed recovery. Review current instrumentation before adding
fields; ordinary frame-read age alone cannot distinguish network delivery from
local dispatch starvation. Do not label the 626ms observation TCP retransmission
without socket evidence. Do not weaken protected input checks or combine entry
operations merely to chase this outlier: observed B entry is below 6ms.

This evidence update changes no runtime, protocol, installer, API, database or
runner. Validation checks revision isolation, event identity and histogram
conservation. Runtime tests, rebuild, native PIN automation and controlled
performance trials are skipped because this commit only records existing
exports and the operator's report. g173 remains the packaged generation.
