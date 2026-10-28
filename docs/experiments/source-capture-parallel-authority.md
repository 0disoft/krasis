# Parallel source capture authority

## Physical observation, 2026-10-28

Compare g475 (`b9fbb079`) with g476 (`2396cc54`). Collection IDs are
`241d12d72a850ff9b978f1a48179a2fb` and `29b1aae71c08c7d7635a04de9cc8f2f5`.
Filter exact builds, deduplicate log segments by source/run/sequence/code,
and match the unique exchange tags. Each cohort has 12 complete successful
A-source/B-installed-receiver profiles. Retained profiles have a fixed sample
limit; these results do not describe every physical crossing or display latency.

| Software span | g475 median / max ms | g476 median / max ms |
| --- | --- | --- |
| A request to grant | 67.7 / 347.2 | 66.0 / 397.3 |
| A capture activation | 17.6 / 25.0 | 63.0 / 294.7 |
| A first offer since begin | 79.3 / 362.3 | 247.7 / 602.4 |
| B ready wait | 37.0 / 328.9 | 36.7 / 369.9 |
| B first dispatch | 7.3 / 194.5 | 2.7 / 5.6 |

Both services/input health remain READY; neither exact-build cohort reports
recovery failures. The user reports the boundary cursor no longer remains on B,
but more stutter. A's activation response wait is 61.8 / 276.9 ms median/max.
Its capture boundary authorization spans each reach the 10-100ms bucket in four
g476 profiles, versus none in g475. Foreground activation also rises from three
to ten profiles in that bucket. These nested spans cannot be added to each other.
The collection does not establish that g476 caused every increased wait.

## Change and controlled verification

`authorizeSourceExecution` previously waited for the service's full proof and
then the agent's full proof. They are independent, read-only queries with their
own channel-owned native lifetimes. Query both concurrently, within the same
one-second budget, and join both bounded callers before returning. Preserve
service-error precedence, fresh checks at each boundary, capacity limits, the
reserved heartbeat slot, and fencing/handle retention for unfinished queries.

A fake-clock regression uses two 600ms proof queries: the old sequence exhausts
the shared 1s budget; the parallel implementation succeeds in 600ms. Repeating
the call verifies no cached proof is reused. Two 1200ms queries still return at
1s and retain both owners until the queries retire. Either owner's denial still
rejects execution. This is a controlled query-wait improvement, not a claim of
600ms physical handoff latency or resolution of foreground-window delays.

After installing g477 on both laptops, collect fresh physical round trips to
measure the actual effect. Keep g476's cursor correction. Rollback requires a
new generation containing the previous serial authority implementation.
