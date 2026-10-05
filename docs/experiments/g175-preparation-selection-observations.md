# g175 preparation-selection observations

Revision: `7c2b643ca04d142a9a83dfd612d791d73530f014`.
These are operator diagnostic exports, not a controlled performance comparison.

| Export | Created UTC | SHA-256 |
| --- | --- | --- |
| A | 2026-10-05 13:48:15Z | `B795D948944612981BBC22A7116280DAEB78AB5AAC84B1E3E07D19D2260828FB` |
| B | 2026-10-05 13:48:11Z | `7DF94F3407B1160E36C4D1DE9678EBE476A0A41E2C75278CF0B00A88B73B3C69` |

## Cohort and preparation decision

Each export has twelve profiles: eleven complete `ok`, and one incomplete
profile (A `incomplete`, B `error`). Both exports have unique run/sequence
identities. Histogram sums match frame counts: A 1,560 [1538,22,0,0,0]; B
1,528 [125,1388,15,0,0]. Each export has one incomplete historical log with
zero current-revision events. Do not count the two hosts as twenty-four
independent transitions or match profiles by local ordinal.

B records service-owned decision 1 (`used`) with `prepared:true` in all twelve
profiles, including the failed profile. There is no observed no-candidate,
expiry, exit or claim-loss decision in this cohort. Startup measurement presence
and decision/prepared consistency pass. Prepared-worker absence cannot explain
this cohort's failure, and no preparation-lifetime change is justified here.

## Successful-profile durations

The eleven complete successful profiles on each host yield the following
process-local durations in ms. The failed profile is excluded from this table.

| Host and span | Min | Median | Max |
| --- | ---: | ---: | ---: |
| A RequestToGrant | 29.782 | 33.026 | 134.928 |
| A FirstOfferSinceBegin | 32.240 | 40.482 | 144.009 |
| A CaptureActivate | 5.485 | 10.987 | 35.064 |
| A SourceDeactivate | 12.737 | 14.386 | 27.081 |
| A per-profile worst Dispatch | 0.750 | 1.107 | 4.522 |
| B RequestReadToGrantWritten | 21.127 | 25.522 | 104.497 |
| B ReadyWait | 6.655 | 7.414 | 9.282 |
| B EntryPrepare | 3.476 | 4.658 | 6.041 |
| B per-profile worst Dispatch | 3.752 | 8.125 | 18.087 |

All eleven completed A grant-progress receipts are byte-observed and
unsaturated, each with two decoded reads, 116 bytes and one complete frame.
All grant receipts pass duration conservation and byte-gap bounds. Ten completed
receipts assign their entire elapsed duration to reading; one has 0.591ms
between frames. All have zero measured dispatch duration. The largest reading
duration is 134.928ms. Reading includes blocking, peer response and scheduling;
this does not identify a network defect.

B's first successful profile has AuthorizeTotal 99.726ms, PeerAuthorize 6.413ms,
ConnectTotal 11.482ms and ReadyWait 8.558ms. The outer duration remains large
despite low measured inner spans. Do not sum nested spans, subtract cross-host
measurements or attribute the residual to a specific function without further
evidence. Both grant delay and pre-grant coordination remain investigation
targets independently of worker readiness.

## Failed transition and recovery

B records `injection_failure` for `entry` at 13:47:34.9159735Z, followed by
`installed_input_cursor_read_failed` and an `injection_failure` for `rollback`
at 13:47:34.9165607Z. Both injection receipts have OUTCOME_UNKNOWN and unknown
counts. The failed receiver profile has no input frames, decision 1, prepared
age 591.678ms, ReadyWait 7.520ms and EntryPrepare 5.753ms. It never writes a
successful grant. A has one partial grant receipt: 43.220ms, zero decoded
bytes/frames, `complete:false`.

The cursor-read event is later than the entry failure and is consistent with
the rollback path. It is not evidence that the initial snapshot read caused
the entry failure. Current logs do not distinguish invalid/clipped entry
position, desktop activity, cancellation or worker-channel failure. Do not
report a specific native error, PIN trigger or root cause as confirmed.

B subsequently records `reconnect_injection_not_applied`, and A records
`reconnect_remote_eof`. Each host has a later CONNECTED, followed by no failure
or reconnect event before export: about 39.6 seconds on A and 35.5 seconds on B.
B's earlier network timeouts and disconnect request precede this connection
and are kept separate. Whether the operator directly changed the connection
during this failed transition was not reported with the exports.

## Next bounded work and validation

First distinguish the initial entry failure from rollback failure using fixed,
privacy-safe stage/reason diagnostics. Preserve unknown outcomes, worker-owned
recovery and current desktop/session/image checks; do not replay uncertain
cursor writes or fall back to ordinary input. Keep preparation expiry unchanged.
Then inspect the independently observed authorization/reader delay.

Validation: exact-revision isolation, unique event identities, histogram/frame
conservation, decision invariants, grant duration conservation, byte-gap bounds,
failure chronology and `git diff --check`. Tests, rebuild, PIN automation and
controlled benchmarks are skipped because this is an evidence-only change.
No API, protocol, database, installer, runner or security-policy change.
