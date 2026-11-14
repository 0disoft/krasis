# g537 paired crossing: reply-gate failure absent, activation outliers remain

Collected both notebooks after the user's completed crossing trial. Exact build
`548204dc691fe001abef38d2b46fd72f44ac6137`, collection
`354b4d94aad3b6b195621ba4e084cba2`; detailed figures and sanitized evidence hashes
are in the adjacent JSON report.

## Connection and heartbeat result

Both notebooks stayed on the same authenticated connection for about 324 seconds
after the g537 installation. Service/input health were READY, grants PRESENT,
and neither service restarted. No new fault, heartbeat failure, reconnect or
recovery-required event was retained after that connection. A's 21 sampled
heartbeat reply-gate measurements and B's 20 were all below 1ms. A's sampled
complete heartbeat response waits were below 100ms, not all below 1ms.

The earlier g536 source exit recorded reply_gate >1s and a sampled
source-gate-action-write holder. That failure pattern did not recur in this
trial. This is bounded observation, not proof against all future stalls; action
credit waiting has no separate production duration counter. B's four acquisition
timeouts and RECONNECTING record occurred during the paired upgrade before the
current connection, while A had not finished upgrading. They are not trial
reconnections.

## Latency

| Local measured interval | Samples | Median ms | Maximum ms |
| --- | ---: | ---: | ---: |
| A capture activation | 14 | 18.888 | 236.627 |
| A request to grant | 14 | 38.619 | 63.960 |
| B worker readiness wait | 14 | 24.114 | 25.766 |
| B first input dispatch | 14 | 1.855 | 4.100 |
| B worst input dispatch per entry | 14 | 4.621 | 144.752 |

There were 15 entry requests/admissions and 15 ready input leases; 14 retained
complete source/receiver profiles match by exchange tag and all end OK. The last
entry is beyond the second diagnostic window's 12-profile limit; its detailed
latency is unavailable. Statistics do not represent every individual input.
Source-to-first-offer median/max were 62.283/340.683ms; this ends at local queue
offer, not the peer's physical display. TCP RTT samples were median 5.550ms,
max 16.201ms. Never subtract A and B wall clocks, which differ by several seconds.

The worst A capture is primarily local activation: 230.481ms inside the worker,
including a 95.860ms foreground request. The same operation's request delivery
was 0.066ms, read authorization 4.090ms, dispatch 0.492ms, reply admission
0.076ms, reply delivery 0.079ms and caller resume 0.793ms. These are software
spans; none alone proves CPU execution time. Native visibility/focus work and
scheduling remain to distinguish. A runtime sampling also recorded timer lateness
up to 142.231ms and sampler lateness 373.340ms; these separate interval maxima
must not be added to capture latency.

B's worst 144.752ms dispatch contained 114.444ms of protected apply work. Its
native-child exchange recorded request-to-read 26.968ms, child handler 35.073ms,
reply-read delay 10.361ms, and parent completion 42.748ms. Actual SendInput
submission was 15.583ms nested within that operation. Service response transfer/
app-read resumption was 25.225ms. Thus SendInput alone does not explain this
outlier, nor do these records prove a particular scheduler or pipe defect.

The next latency target supported by this trial is A's local capture activation,
especially native window visibility/focus work. The shared reply-gate failure is
absent in this sample; remaining stutter has separate activation/input outliers.
g536's 38 complete retained profiles had capture median/max 19.276/256.334ms,
but load, sample count and observation window differ: no controlled overall
latency improvement is established.

## Validation and scope

Verified live CLI doctor healthy/CONNECTED, exact paired build, support health,
unchanged service starts, matched exchange tags, measured-mask handling, and
frame histogram totals. Current agent logs report complete; historical control
previous logs are incomplete and are not treated as complete crash history.

No runtime, API, DB, runner, dependency or source-of-truth contract changed.
No test/build/CI or reinstall was run: this task inspects real-device evidence.
Long-duration/load-matched trials remain unperformed.
