# g345: paired entry and lock/PIN trial

The user reported finishing the requested idle-first-entry, repeated crossing,
and B lock/PIN/login trial. Paired collection
`ca1f07eef19b7883ee62ca68bb1124dc` was received on 2026-10-19 at approximately
08:59 UTC. Both sides report build
`12605ac0d11adef67265fb47d3c3eb4124b2de67`.

## Observed stability

There are 14 source summaries on A and 14 receiver summaries on B, matched by
their exchange tags. All end `ok` and none is incomplete. Agent logs are
complete; one previous control/UI log on each side is incomplete. The active
agent run and connection identifiers remain unchanged through the trial.
No new fault, heartbeat failure, or reconnect is recorded after 08:54 UTC.
Earlier B acquire-network timeouts at 08:52-08:53 precede this trial.

B records session lock and desktop transitions at 08:57:48-08:57:55 UTC.
Both latest input-health receipts are `READY / complete`; both service runtime
receipts are `READY`, with no last worker failure. A's live doctor check is
`healthy / CONNECTED / PROTECTED`, with matched running service and build.
These observations confirm this trial's continuity, not indefinite stability
or a measurement of subjective smoothness.

## Remaining entry latency

The 14 matched source request-to-grant durations range from 33,424 to 174,220 us,
with median 40,317.5 us. Eleven are below 45 ms. The remaining three are:

| Exchange prefix | A request-to-grant | B preparation evidence |
| --- | ---: | --- |
| `3c1e6941` | 174,220 us | Expired candidate; readiness wait 158,838 us |
| `9741bd6a` | 70,838 us | Prepared candidate used; preparation lock overlap 34,436 us |
| `74a98c54` | 91,825 us | Prepared candidate used; preparation lock overlap 50,356 us |

The expired-candidate exchange has B authorization 161,997 us, including
connection setup 160,804 us. Startup reports service launch 17,693 us, child
connection 120,909 us, and bootstrap readiness 9,597 us. Installed validation
is 58,318 us within child startup, not an additional serial contribution.
Every matched receiver pipe-open profile has one successful attempt and zero
recorded busy/missing retries or timer waits. A zero microsecond syscall
measurement is rounded timing, not evidence that the operating system did no work.

The fixed two-second candidate lifetime explains the cold fallback decision;
it does not by itself establish which hint, return, expiry, or admission race
should change. The next bounded investigation is preparation freshness and
entry contention, preserving the current lifetime, launch cooldown, independent
authentication, cleanup completion, and prohibition on replaying input.

## Local Go trace

The opt-in A recorder saved a 2,320,365-byte snapshot at
`2026-10-19T08:57:34.631587Z`, following the 174-ms source summary. Offline
analysis used a copy so neither live snapshot slot was held open.
SHA-256: `e015812cebb2fb236d9f527784b99aa5f2e4331bed5f9d146116af156307dd2d`.

`go tool trace -d=parsed` covers 7.011550553 seconds and contains six paired
GC stop-the-world ranges: maximum 527,162 ns and total 1,205,539 ns. A long
stop-the-world pause is not observed in this sample. This local A trace cannot
exclude B GC, concurrent GC overhead, or operating-system scheduling costs.
The receiver profile independently places the long entry wait in cold startup.
No raw trace or input/PIN/clipboard payload is included in this report or support
transfer.

This commit records field evidence only. It changes no executable, protocol,
authority rule, generation, or deployment. Validation was paired collection,
live doctor, exchange-tag matching, numeric summary calculation, and native
trace decoding; product tests/build were not repeated for this report.
