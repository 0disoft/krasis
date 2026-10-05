# g174 grant-progress observations

Revision: `234dcd59f72341368c8e02c47098d6ca31b04f3b`.
This records the operator's latest diagnostic exports, not a controlled
performance comparison or a new runtime change.

| Export | Created UTC | SHA-256 |
| --- | --- | --- |
| A | 2026-10-05 13:22:54Z | `11500AC50168E53682FD6C5A7378FCB8E2F9F09941D44CF08C1CF30372925C9D` |
| B | 2026-10-05 13:22:51Z | `2E6DE85EAB5468D3861FCCC75347A0FFD668B0AC30A1644AF3D9762C0FD7F438` |

## Connection and profile boundaries

Filter by the exact revision. Both exports contain twelve complete profiles
ending `ok`: A source, B installed receiver. Post-connect observation is about
69.8 seconds on A and 66.7 seconds on B using each host's own timestamps, with
no subsequent reconnect, failure or session stop. B's two RECONNECTING events
and its stopped/disconnected events precede the final CONNECTED. B has forty
lease-ready events. Current-revision events in incomplete historical logs: zero.
PIN-open behavior was not separately reported with this export.

Event identities are unique within each export. Profile histogram sums match
frame counts: A 1,341 frames, [1270,71,0,0,0]; B 1,279 frames,
[134,1132,13,0,0]. Twelve profiles on each side are not twenty-four independent
transfers and do not describe all later traffic beyond the collector quota.

## Observed durations

These are measured process-local durations in ms over twelve profiles.

| Host and span | Min | Median | Max |
| --- | ---: | ---: | ---: |
| A RequestToGrant | 31.893 | 41.385 | 154.068 |
| A FirstOfferSinceBegin | 37.098 | 49.2275 | 172.301 |
| A SourceDeactivate | 17.225 | 28.074 | 173.940 |
| B RequestReadToGrantWritten | 22.027 | 25.7075 | 128.862 |
| B ReadyWait | 6.412 | 7.462 | 113.005 |
| B EntryPrepare | 4.303 | 4.8535 | 6.146 |
| B per-profile worst Dispatch | 2.358 | 11.689 | 17.347 |

All twelve A `grantProgress` objects are complete, byte-observed and unsaturated.
Each has two TLS-decoded reads totaling 116 bytes and one complete frame.
Duration conservation and byte-gap bounds pass. Eleven profiles assign their
whole interval to reading; one assigns 0.541ms to local dispatch and the rest
to reading. A's slowest interval is 154.068ms in reading, including the gap
before its first decoded-byte callback. This rules out a long A local-dispatch
span inside that observed grant interval; it does not identify TCP loss or
distinguish scheduling from waiting for B's response.

B independently has one 113.005ms readiness wait in a receiver profile without
the prepared-worker marker. The eleven profiles with the marker have readiness
waits of 6.412..8.985ms. The slow profile's OpenLock is measured zero, with
preparation overlap measured zero. Its ConnectTotal is 115.874ms,
AuthorizeTotal 123.512ms and request-read-to-grant-written 128.862ms. This local
nesting supports investigating worker readiness, not summing these values.

A also has a 173.940ms SourceDeactivate, containing a 165.700ms focus-restore
measurement. The same export's 60-second runtime summary has a 356.372ms
maximum dispatch and 113.939ms timer lateness. Those maxima are not associated
with a profiled attempt and cannot be assigned to the grant-read interval.

Do not match A and B attempts by their local ordinal, subtract cross-host
durations or infer wire delay from a distribution difference. The g173 626ms
outlier was not observed here; this is not evidence of a controlled improvement.

## Next bounded work

Record the fixed reason why a lease did not use a prepared worker, before
altering expiry, authorization or the preparation schedule. The existing
unbound worker expires after two seconds; current evidence does not distinguish
no candidate, expiry, exit, session change, a claim race or unavailable hints.
Keep session/desktop/image checks and independent lease authorization intact.
Do not extend privileged preparation indefinitely as a latency workaround.

The slow readiness case is the first investigation target. Keep the independently
observed source focus-restore delay as a second target. Socket instrumentation
is still useful if later evidence isolates a read delay that local peer
readiness cannot explain.

Validation covers revision isolation, unique event identities, histogram/frame
conservation, grant-phase conservation and byte-gap bounds. Runtime tests,
rebuild, PIN automation and controlled performance comparisons are skipped for
this evidence-only update. g174 remains the packaged generation; no API,
protocol, database, installer, security policy or runner changes are made.
