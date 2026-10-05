# g177 cleanup and delay observations

Revision: `6cf9b14ea52b5c8112aadf3860ddb8016f754c17`.
These operator exports are observational, not a controlled comparison.

| Export | Created UTC | SHA-256 |
| --- | --- | --- |
| A | 2026-10-05 14:59:24Z | `23D40571AB6E7B04E41C1E6E41747C28AA906C5CD665CF797DE9134DA706C626` |
| B | 2026-10-05 14:59:19Z | `6A8E3E6A06DAB555F603D7FE856ACBD46BB537D28C2CFC296023AE732C723FE2` |

## Successful transitions and observation limits

Both hosts have twelve complete profiles ending `ok`, with no current-revision
events in incomplete historical logs. Run/sequence identities are unique;
histogram/frame conservation passes: A 1,698 [1678,20,0,0,0], B 1,683
[81,1580,22,0,0]. Profiles are capped at twelve attempts within 120 seconds.
They do not represent all subsequent input or twenty-four independent crossings.

A has one CONNECTED and no failure, reconnect or disconnect event in this
revision. B's network retries and manual disconnect precede its final CONNECTED.
After connection, neither host records injection/cursor rollback failure,
foreground activation failure, installed-input connection/heartbeat failure or
reconnect: about 68.1 seconds to export on A, 63.0 on B, each using local wall
timestamps. B has forty lease-ready events.

B's twelve profiles all carry preparation decision 1 (`used`). No expiry or cold
fallback is observed. The g176 rollback `inspect` failure does not recur in this
cohort. However, there is no failed transition or positive rollback receipt here,
so this cohort does not prove that the newly reordered native rollback branch
executed. Hosted regressions establish the corrected order; a forced native
abort and PIN-open reentry were not separately confirmed with these exports.

## Process-local durations

All twelve profiles are successful. Durations below are ms; nested spans must
not be summed or subtracted across hosts, and local attempt ordinals do not pair
wire operations across hosts.

| Host and span | Min | Median | Max |
| --- | ---: | ---: | ---: |
| A RequestToGrant | 31.359 | 33.3025 | 63.285 |
| A FirstOfferSinceBegin | 39.740 | 41.1735 | 71.558 |
| A CaptureActivate | 8.188 | 10.6865 | 68.523 |
| A SourceDeactivate | 13.935 | 22.494 | 73.446 |
| A per-profile worst Dispatch | 0.793 | 1.411 | 2.134 |
| B RequestReadToGrantWritten | 21.287 | 28.9145 | 54.247 |
| B ReadyWait | 6.427 | 7.3505 | 9.243 |
| B EntryPrepare | 3.591 | 4.756 | 6.014 |
| B FirstOfferSinceBegin | 39.066 | 53.8875 | 104.461 |
| B per-profile worst Dispatch | 5.251 | 9.3535 | 57.837 |

All twelve A grant receipts are complete, byte-observed and unsaturated. Each
has two decoded reads, 116 bytes and one frame. Duration conservation and
byte-gap bounds pass. Dispatch duration is zero except one 0.336ms sample;
reading still includes waiting, peer response and local scheduling.

B's worst retained input vector has Dispatch/ClientTotal 57.837ms,
ClientAuthority 15.898ms and ClientResponse 41.939ms. Its matching worker ACK
reports authority 8ms, scope 0ms and apply 31ms, all measured and unsaturated.
Worker durations are floored: scope 0ms means less than 1ms, not no check;
apply 31ms is in [31,32)ms. ClientResponse contains worker work and delivery;
these fields cannot be summed into an independent end-to-end latency.
The apply span can contain multiple native actions including cursor readback;
it does not isolate one syscall or prove a network cause.

The latest 60-second runtime summaries separately contain A queue residence
maximum 57.540ms and dispatch maximum 126.504ms; B injection/dispatch maximum
57.838ms. They are interval maxima, not necessarily the same operation as the
retained vector. Their intervals also include time just before connection.
Do not merge these maxima into a synthetic slow transition.

## Next bounded work and validation

Investigate B's input apply span and its client authority work before changing
worker expiry or network behavior. Keep source capture/restore latency as a
separate target. Preserve per-operation acknowledgment, outcome uncertainty,
authorization, recovery and diagnostic privacy. No improvement or permanent
stability claim is supported by this short, uncontrolled cohort.

Validation: exact-revision isolation, event identity, histogram conservation,
selection invariants, grant duration conservation, byte-gap bounds and staged
`git diff --check`. Runtime tests, rebuilding, forced abort and controlled native
comparison are skipped for this evidence-only document. No API, database,
protocol, installer, runner or security-policy changes.
