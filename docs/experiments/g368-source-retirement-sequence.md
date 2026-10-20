# g368: preserve source retirement after an unsubmitted write

The operator reported recovery-required again after g367. The preserved local
support snapshot is `.git/krasis-improvement/g367-recurrence-20261021/support.json`.
At 2026-10-20 16:51:37Z (October 21 01:51 KST), an idle source heartbeat
response times out, followed by `installed_source_reader_protocol`, failed
source stop and FAULTED. The service remains READY, and its source receipt
reports capture/sender/interrupt joined but retirement delivery failed. The
agent runtime summary also observes timer lateness up to 4203 ms. The origin
of that scheduling delay is unproven; activation is not this failure's trigger.

Candidates, prioritized by impact and bounded implementation cost:

1. Unsubmitted source replies/batches consume wire sequences, making a later
   valid retirement look invalid. Reproduced and fixed in this change.
2. Attempted/partial write failures allow more frames on an uncertain stream.
   Fixed with the same writer boundary: reject subsequent writes.
3. Scheduling stalls and other missing-retirement paths can still stop
   automatic recovery. Investigate separately without relaxing active safety
   deadlines or treating EOF as proof of native cleanup.

A native named-pipe regression expires the first reply deadline before
WriteFile submission. Before the fix, the unsent reply consumes sequence 11,
so the distinct joined-retirement frame would use 12 after the last delivered
sequence 10. Both this test and the attempted-write fencing test fail on the
old source. This is a reproduced escalation path consistent with the new
protocol observation; the log alone cannot prove its exact branch.

The writer now preserves sequence only when no write was submitted or setting
its deadline failed. Its existing one-second write bound is unchanged. A
partial or attempted failure poisons the stream even if Write returns zero;
no later reply, boundary batch or retirement can append bytes. The shared
helper covers ordinary packets and bounded pointer batches. It never retries
the failed operation. Native joins, exact binding/sequence checks, active
two-second safety limits and retirement requirements remain intact.

No public API, DB, dependency, repository hygiene or CI runner changes.
The operator's ongoing authorization permits applying the verified g368 trial
to both PCs without another approval. No Git push or public release.

Local tests pass for inputworker, residentagent, agent, peer and localapi.
Vet passes with the existing unsafeptr exclusion. Regression coverage uses a
real native pipe for an unsubmitted reply and batch, and rejects further
retirement/batch writes after zero-byte attempted and partial failures.
Package and installed acceptance results are recorded separately below.
