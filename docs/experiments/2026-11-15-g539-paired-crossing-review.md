# g539 paired trial: native reply tail reduced, no new reconnect observed

After the user reported completed crossings and no visible issue, collection
`40d88707c621af17cf1fb3442830d788` captured both notebooks on g539 revision
`d9cb745a38d0126e27e71f8195d8817b6211faad`. Live CLI doctor reports
healthy/CONNECTED with matching CLI, agent, UI and protected installation.
B is ONLINE on the same revision; its update transfer is IDLE.

## Latency comparison

All 13 available A-source/B-receiver exchange-tag pairs are complete and OK,
with no unmatched profiles. B processed 1,297 frames across those pairs.
There are 16 boundary admissions; latency statistics cover only the 13 completed
profile pairs, not every admission. Frame histogram sums were checked.

| Local measured interval | g538 median / maximum ms | g539 median / maximum ms |
| --- | ---: | ---: |
| B native reply ready to parent first read return | 0.179 / 337.190 | 0.127 / 2.215 |
| B worst input dispatch per entry | 19.750 / 393.157 | 5.229 / 44.978 |
| B first input dispatch | 2.292 / 6.739 | 1.966 / 3.574 |
| B worker readiness wait | 26.592 / 81.909 | 23.909 / 25.233 |
| A request to grant | 43.969 / 213.021 | 41.514 / 45.612 |
| A capture activation | 13.390 / 61.325 | 12.888 / 67.755 |
| A foreground request | 7.785 / 36.725 | 7.106 / 47.028 |

The former 337ms native reply tail is absent from the retained worst-input
samples: the maximum is 2.215ms, 99.3% lower. Worst dispatch maximum is 88.6%
lower. This supports the IOCP reader change, but does not prove the original
entire stall was caused by synchronous reads. g538 covers 20 complete pairs and
4,961 receiver frames, versus 13 and 1,297 now; workloads and CPU pressure differ.
Native stage statistics describe each entry's worst-dispatch frame, not every
ACK. Reply-ready to first read combines encoding/write, delivery and resumption.

## Remaining tails

The worst B dispatch is 44.978ms. Its nested native child handler is 29.010ms,
with SendInput submission 2.763ms inside that handler. Request delivery is
3.285ms, reply-to-read 2.050ms, and parent request preparation is 6.103ms.
The handler remainder is not established as SendInput or a particular OS call.
The maximum reply-to-read of 2.215ms belongs to a different entry.

A activation maximum remains 67.755ms, including 61.941ms worker activation and
47.028ms foreground request. A medians are slightly lower but complete-pair
maxima increased; an A tail improvement is not demonstrated. The same entry's
first-offer-since-begin is 189.904ms, which includes availability of source input
and must not be described as physical cursor latency. Its sampled TCP RTT is
53.344-59.345ms, compared with a trial median 9.308ms. This does not isolate
network transit for the handoff. A sampler lateness reaches 442.523ms in a
separate runtime sample; it is not a matched causal explanation of this entry.
Nested intervals must not be added; device wall clocks must not be subtracted.

## Reconnection and recovery

The current shared authenticated connection is unchanged from the installation
receipt. A retains 309.146s and B 310.380s since their locally logged connection:
no new fault, timeout, poisoning, reader-retirement, disconnect or reconnect is
retained in that interval. Both service/input states are READY, grants PRESENT,
service start times unchanged, and recovery last-failure records unchanged.
No new error-1066 service stop or administrator repair is evidenced.

The five B acquisition timeouts during installation preceded this connection
and remain documented in the installation report; they are not trial failures.
About five minutes cannot verify the clean active capture-retirement fix at the
one-hour lease boundary or establish long-duration freedom from recurrence.
Historical control.previous logs are incomplete; this is not a full-history audit.

## Verification and scope

Checked exact live build agreement, shared connection, evidence SHA-256 hashes,
event deduplication, unique exchange pairing, measured-zero handling, histogram
sums, service starts, recovery ledger comparison and native timing definitions.
The adjacent JSON preserves sanitized measurements and selection details.
No further runtime change or reinstall is justified by this successful short
trial. Source tests, build, remote CI and push were skipped because only evidence
reports changed; prior runtime validation is recorded in the g539 design report.
No source-of-truth contract, API, DB, dependency, hygiene or runner changes.
