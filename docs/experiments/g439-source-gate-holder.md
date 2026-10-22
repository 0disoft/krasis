# g439: sample the source writer holding a failed reply gate

g438 collection 120126db5417f4da8c596878309c83c4 retains an A source exit at
14:08:33Z with heartbeat stage reply_gate and lifetime slow bucket ge1s. Later
collection 58c40951fe86cceb4c8dc2fdd714fb5c still shows both services READY,
without another source exit. These records identify gate contention, but cannot
identify the writer or distinguish its presence check from its pipe write.

Publish an atomic closed origin/stage only while the source writer owns the
existing gate. Origins are boundary, action, reply, return, and retirement;
stages are prepare, presence, and write. Clear the sample before unlocking.
If a completed reply, heartbeat, or retirement fails gate admission, preserve
the currently observed holder through its existing failed final report as
source-gate-ORIGIN-STAGE. A cleared/unpublished owner is source-gate-unobserved.
No payloads, clocks, paths, input details, or free-form labels are recorded.

The sample is the holder observed after failed admission. It is not proof that
the same holder occupied the gate for the entire wait, nor a measure of its
native query duration. The short publish/clear windows remain unobserved rather
than attributing a released owner. No polling or detached lock waiter is added.

Tests stall a real synthetic writer in presence validation and each origin in
pipe writing, expire a competing heartbeat, then require the original write to
complete with its unchanged sequence and no poisoned stream. Verify that the
holder clears, the sampled operation survives the authenticated report, later
retirement cannot replace it, and native cleanup uncertainty still takes
precedence. Closed-label validation and byte operation capacity are checked.

Passed: inputworker (22.525s), logindiag, productlog, and residentagent via
`go test -timeout 90s` with the portable pair fixed-owner fixture excluded to
avoid conflict with the installed session. `git diff --check` passed. No race
detector, hosted CI, physical-input acceptance, or long soak in this change.

No admission, authorization, timeout, retry, input delivery, or recovery policy
change. Private diagnostic codes are appended; frame layout, public API, DB,
dependencies, runner, repository hygiene, and product/architecture specifications
are unchanged. Local main only; no push. This is diagnosis, not a latency fix.

Packaging and paired rollout results will be recorded separately. Next identify
the holder in a fresh failing sample before changing its behavior.
