# g436: keep final source-write presence outside the heartbeat gate

g435 paired collection 2e0698af5b48c3f86369ebe55a24774a shows A's
2026-10-22T13:14:38.6663308Z aggregate client response and reply-gate maxima in
lt1s (100ms to below one second), while presence wait/native maxima are lt1ms.
Both sides remain READY, with no source exits or post-connect faults. Independent
15-second maxima cannot identify which writer caused that particular wait.

Code inspection finds a remaining avoidable gate owner: ordinary source writes
perform full authorization before TryLock but run final presence in a second
native query after acquiring the wire gate. A delayed query's admission or
scheduling can therefore hold heartbeat replies behind an input write.

Use one mandatory input-lane query for the full proof followed by final pinned
owner presence. Admit only through immediate TryLock. Gate contention discards
both proofs; the writer waits without authority and repeats both before retrying.
The active generation is still checked under the gate before sequence/bytes.
Input/read authorization outside this writer is unchanged. Heartbeats retain
independent presence admission; advisory batches, completed-command replies and
joined retirement retain their previous checks. No cached proof, optional input
query, wider deadline, altered capacity, extra goroutine or failed-write replay.
An ordinary write now needs one native query owner instead of two.

Regression tests stall the first ordinary write's final presence and require a
heartbeat to arrive first with consecutive wire sequences, then release the
write. A contended admission must repeat both full proof and final presence and
reject the now-revoked owner without any bytes. Existing owner/generation loss,
caller cancellation, retained native-query ownership and retirement tests apply.

This removes a reproducible blocking path, not a causal attribution of the live
100ms+ sample or a claim that intermittent ge1s latency is resolved. Next compare
fresh paired timings and inspect remaining wire/write scheduling if needed.

No private/public frame layout, public API, DB, dependency, runner, hygiene or
product/architecture source-of-truth change. Local main; no push/ref update.

WITHDRAWN in g437: A had two live source-write-timeout / heartbeat failures at 13:29:32Z and 13:30:11Z on g436. Restore g435 writer/query behavior before further experiments. Causality is unproven; do not claim the gate change fixed runtime latency. Paired evidence: 6f697649892a6df3ff431a73bde2a0e5. g435 inputworker/productpanel validation remains applicable to the restored code; packaging validates the new generation.
