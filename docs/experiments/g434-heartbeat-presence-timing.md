# g434: profile acknowledged heartbeat presence checks

g433 stays CONNECTED with no source exits, but A's 2026-10-22T12:43:08.5530425Z
heartbeat response maximum reaches ge1s. That 15-second aggregate proves at
least one slow acknowledged response, not a specific native-call duration or
a disconnect. The old source exit stages cannot explain a slow success without
deliberately ending the session. Baseline paired collection:
3547d7b797d02185a72b4ef1d07b5c46.

Carry three closed timing buckets on an exact heartbeat reply: cumulative
presence wait, cumulative presence native-check interval, and reply-gate wait.
The first includes query-lane admission, goroutine scheduling and result
publication/observation. The second covers the source presence check invocation,
including any descheduling within it; it does not isolate kernel CPU or a single
Windows syscall. Sum service read, agent read and reply presence for one serial
heartbeat. Actual pipe write/client receive remain outside these worker buckets.

Only heartbeat-owned presence queries allocate optional private timestamps.
Native goroutines never write the serial heartbeat aggregate: the caller merges
after done closes and only while its context is still valid. Expired native
queries retain their handles/slots and cannot mutate a later heartbeat sample.
The independent watchdog, input and advisory checks are not profiled here.
Reset per request; retain separate lifetime exit-stage diagnostics unchanged.

Use the existing three fixed timing bytes in the private 64-byte source frame.
Activation retains its original semantics and zero first timing byte. Heartbeat
replies admit all three bounded bytes; all other operations reject them. Source
images are pinned to the same build, so this does not require a network protocol
change. Only a matching authenticated reply contributes timings. Emit the new
worker_presence_wait, worker_presence_native and worker_reply_gate codes through
the existing 15-second aggregate sampler and sanitized export allowlist.
No per-request logging, new timer/goroutine/query, raw trace or input payload.

Tests separate 200ms queued waiting from 30ms native execution, retain a fresh
query after contention, preserve native ownership after timeout without late
sample mutation, reject foreign reply timing, round-trip the heartbeat bytes,
reject out-of-range bytes and unknown export phases. Inputworker PASS (22.422s),
productlog PASS (2.078s), residentagent PASS (7.288s); final focused tests PASS
(0.107s). Packaging/application results are recorded separately. Portable pair
fixed-owner fixture is skipped to avoid conflicting with the real session.

This is diagnostic work, not a repair claim for the remaining slow response.
Next correlate a ge1s client window with the three worker phases. No deadline,
authority/fencing/retirement, public API, DB, dependency, runner, hygiene or
source-of-truth specification change. No remote push/ref update.
