# g178 worker apply decomposition observations

Product revision: `881768fab08ebf9265e56341f1d71e7b793fbe15`.
These operator exports are an observational cohort, not a controlled comparison.

| Export | Created UTC | SHA-256 |
| --- | --- | --- |
| A | 2026-10-05 15:41:19Z | `F6FCD5986D7AB8623A3C1F62974D142116DCD33B6116A2302174C42343BBA7E0` |
| B | 2026-10-05 15:41:15Z | `E52B04D17BC22902C32BA6CC57EC91C1513F30DE98938DF3EEA9AE7680DA96D0` |

## Cohort and connection state

Each host retains twelve complete handoff profiles ending `ok`. Event identities
are unique within each host, and histogram conservation passes: A has 1,167
frames [1162,5,0,0,0]; B has 1,154 [17,1125,12,0,0]. The collector retains at most
twelve attempts within 120 seconds. The hosts' local ordinals do not independently
identify the same wire operation or twenty-four independent crossings.

No current-revision events occur in incomplete historical logs. After the final
CONNECTED event, neither host records a failure, disconnect, timeout or reconnect
through export: approximately 82.6 seconds on A and 78.6 on B, using local wall
timestamps only. B records 61 lease-ready events across the revision. Its earlier
28 network acquisition timeouts and manual disconnect precede final connection;
their cause is not resolved by these profiles.

All twelve B startup receipts report preparation decision 1 (`used`). There is
no failed transition or positive cursor rollback receipt in this cohort, so the
native rollback branch introduced in g177 is not directly exercised here.

## Worker-local input application and cursor snapshot

`WorkerApplyProfile` splits the existing `apply` function (slot 0) from subsequent
cursor readback (slot 1), inside the existing authorized desktop scope. The source
boundaries are `internal/inputworker/apply_detail.go`, `worker.go:apply`, and
`internal/handoff/worker_apply.go:ValidForWorker`. Native input methods reach
`internal/wininput/sendinput_windows.go:sendInputs`; this receipt does not isolate
one syscall. Operation kinds, input values and cursor coordinates are not retained.

B has 24 retained detail vectors: twelve first and twelve worst. All satisfy mask,
range and containing-span bounds, with no saturation. A's source vectors have no
worker detail, as expected. First and worst may select the same input; these are
retained vectors, not a distribution of all input events. Worst is selected by
dispatch duration, not by the individual worker part.

| B selection and part | Measured vectors | Min (ms) | Median (ms) | Max (ms) |
| --- | ---: | ---: | ---: | ---: |
| First apply | 12 | 0.511 | 0.5785 | 1.562 |
| First snapshot | 12 | 0 | 0 | 0.569 |
| Worst apply | 12 | 0 | 0.944 | 18.602 |
| Worst snapshot | 11 | 0 | 0 | 1.879 |

A measured zero is below microsecond resolution, not an absent stage. An absent
presence bit is unobserved and excluded from the corresponding distribution.

B's slowest retained worst vector (local attempt 11) ends `ok`:

- Dispatch: 22.451ms; ClientTotal and ClientResponse: 19.119ms each.
- ClientAuthority: measured 0 microseconds.
- Worker authority/scope/apply: measured 0/0/18ms; these are floored milliseconds.
- Worker apply detail: 18.602ms, measured mask 1, no saturation.
- Cursor snapshot: not measured on this operation, not interpreted as zero latency.

This same-operation receipt locates the dominant measured worker work in `apply`,
not subsequent cursor readback. The 31ms worker span seen in g177 does not recur
in the retained g178 vectors. Different workloads and retention prevent a claim
that g178 improved performance or explains the older 31ms sample.

Other B worst vectors retain authority delay up to 9.189ms (attempt 12), with
apply 0.574ms and response 1.171ms on that operation. Authority is therefore a
separate delay candidate. Do not sum maxima from different operations or derive
a scheduler residual from nested spans.

## Entry and return remain separate targets

| Process-local span | Min (ms) | Median (ms) | Max (ms) |
| --- | ---: | ---: | ---: |
| A RequestToGrant | 29.617 | 36.562 | 117.754 |
| A FirstOfferSinceBegin | 33.006 | 41.5315 | 121.233 |
| A CaptureActivate | 6.031 | 8.6375 | 37.167 |
| A SourceDeactivate | 37.581 | 44.276 | 81.814 |
| B RequestReadToGrantWritten | 23.349 | 32.3825 | 72.906 |
| B ReadyWait | 7.481 | 8.2495 | 10.887 |
| B EntryPrepare | 4.237 | 5.2445 | 7.416 |
| B FirstOfferSinceBegin | 41.741 | 54.241 | 121.419 |

These spans are nested or on different processes. They must not be summed or
subtracted across hosts. First-offer timing is not proof of pixels presented.

## Next bounded step and validation

Investigate the native application path before removing cursor readback. Add a
bounded same-operation duration around native input submission to distinguish
`SendInput` call time from its containing `apply` span. Preserve authorization,
ACK outcome certainty, operation order and recovery; do not resend inputs or
weaken process ownership checks. Source deactivation and authority work remain
separate targets rather than being folded into one purported root cause.

Validation: exact-revision isolation; host-local event identity; histogram,
grant-duration and byte-gap conservation; preparation selection invariants;
worker-detail presence/saturation/range checks and `ValidForWorker` bounds;
same-vector review; `git diff --check`. Procedure selection: desktop-app and
performance-measurement-integrity-review.

Runtime tests, rebuilding, new native PIN/rollback trials and controlled performance
comparison are skipped because only this evidence document changes. No product
version bump, API, database, protocol, installer, runner or security-policy change.
Short observation and bounded vector retention leave intermittent delay and
long-term stability unresolved.
