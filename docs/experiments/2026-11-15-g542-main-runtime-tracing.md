# g542: identify main-agent scheduling and grant wait stages

## Existing evidence, not a new performance claim

g541 trial collection `0d316fb530aac96e8844a4107e32a07a` contains six A GC trails
that were not included in the earlier compact crossing report. In the trail
triggered at diagnostic-relative 207261832us, the 196812512..207261832us window
has weighted system busy/total about 98.95%. It overlaps the first physical
entry and the period preceding the later rapid crossings, rather than proving
every later grant wait had the same cause.

One window records metrics.Read 1078830us, followed by a total/stopping GC
upper-bound bucket observation. OS reads in that trail reach 450592us and
sampler lateness 1081255us. Both total and stopping histograms have large
entries, but their independent bucket counts cannot be subtracted to infer
exact GC work. Other windows are delayed with no GC event at all. This supports
investigating scheduling/native wait as well as GC; it does not prove GC compute
or one named Windows API caused the user's input stutter.

Runtime GC trail stamps and slow-dispatch stamps are relative to
runtimeDiagnostics.started. Event elapsedMicros is relative to Logger.started.
In these retained trails their offset is about 2.4505s. They must not be treated
as one clock. TriggerObservedAt and TriggerMicros anchor a GC trail locally;
cross-device wall clocks are still never subtracted for network transit.

The same g541 maximum grant exchange is 107707us on A, versus B's process-local
request-read-to-grant-write 32188us and sampled TCP smoothed RTT 66027/66140us.
Those values do not separate network transit from A's runnable/read resumption.

## Instrumentation slice

Add fixed trace regions for actual wire read, read enqueue, writer permit wait,
writer operation, seven decoded lease/preparation kinds and unknown fallback,
plus independent metrics.Read and OS sampler phases. Go's execution trace
already records syscall/net blocking, runnable transitions and GC stopping;
regions identify which operation those states belong to. Labels contain no
untrusted input and do not activate tracing or change JSON/wire schemas.

Cancellation, native/query ownership, validation order, semantic write order
and error chains remain unchanged. Existing opt-in recorder TTL, snapshot
limits, ACLs and terminal-failure reservation remain unchanged. Off-trace writer
regions must add zero allocations, checked by AllocsPerRun.

This generation improves attribution only. Do not claim that main-runtime or
grant latency is solved. A new trace and physical crossings are required before
choosing a performance change. Install the committed g542 build on both devices
and restore CLI connectivity; use a locally opt-in A trace for the next trial.

Source of truth: docs/architecture/02-runtime-flow.md. No API, DB, dependency,
repository hygiene or runner changes. Push/remote CI/public release are outside
this request. Linux native smoke is not needed for fixed portable trace labels;
Windows product paths and package verification remain required.

## Validation and paired delivery

Committed instrumentation revision cb12c135432f88668e4bfcdc5b40d4db7ad1fc08
passed peer, residentagent, runtimestall, handoff and agent tests, scoped vet
with the repository's -unsafeptr=false policy and diff checks. Packaging passed
111 frontend tests, native UI build, input installer build, workspace wire
transfer test, icon/single-copy checks and app setup verification. No dependency,
public API, DB, repository hygiene or runner changes. Remote CI/push were not
requested; native Linux UI smoke was not run for these portable annotations.

B received exactly one 29230102-byte send, ID
4cd835bb9bc1ffac3faa7118684ddf00. The SEND reply was lost while B replaced its
agent. Read-only status confirmed UPDATED and the exact g542 revision before A
installation; no resend occurred. A installation result was 0, stage complete,
recovery not_needed. CLI app-connect receipt 3deabe5454bceef2bb1c4b7381d5981f
confirmed the initial connection. After A replaces its update manager, waiting
on the old transfer returns transfer_replaced; this is not a failed B install.
Both doctor and peer inspection verified matching UI/agent/launch/protected
installation, READY input/service, ONLINE g542 and IDLE update state.

## Traced failure: instrumentation itself is a confound

A run 99c5fdc3038c59bc56ed4419b72ce7dd enabled the 15-minute opt-in recorder at
13:28:12Z. The user then encountered the agent-preparation-failed message.
The terminal snapshot saved at 13:32:49Z is retained locally and parsed with
the same Go toolchain. Its hash and fixed-region durations are recorded in
2026-11-15-g542-main-runtime-trace-review.json; raw traces and the 163 MB decoded
event stream remain ignored local evidence, not source or support uploads.

The trace observes GC sweep-termination stop-the-world lasting 7095.813219ms.
The last observed processor to become idle, P8, stops 1.036763ms before this
range ends. Its G54 stack is inlineUnwinder.next -> fpunwindExpand ->
dumpStacksRec -> traceStackTable.dump -> traceAdvance. This is evidence of a
trace-stack-expansion delay in the traced run, not a 7-second heap scan or proof
that ordinary g541 sessions suffer the same trace-specific path. The allocation
that triggers this GC is a local HTTP/TLS handshake; trigger identity alone
does not establish allocator culpability or memory leakage.

The new regions separate the sampler's worst 7192.053880ms OS-counter interval
into 6821.616286ms Runnable, 370.423476ms Syscall and 0.014118ms Running.
It overlaps the GC stop. Calling this a 7.2-second Windows API execution would
therefore be incorrect. metrics.Read itself is at most 0.728995ms in this trace.
Fifty completed writer permit waits are at most 0.006559ms; a separate 759.903ms
writer operation spends 751.664ms waiting in select. These are nested spans and
are not added or subtracted across unrelated operations. No complete grant
read region is retained here, so this snapshot cannot allocate a full physical
entry delay between network transit and receive resumption.

Source deactivation fails at 13:32:47Z. The first source reader exits with all
native owners joined and clean_sent; a replacement reports not_started/eof and
delivery_failed. Recovery reaches its deadline and the source hook faults.
The protected service remains READY, with no new service-1066 repair in this
evidence. Do not describe this incident as no recurrence or as a clean trial.

## Recovery and remaining work

An attempted identical-package reinstall stops during recovery_registration
preflight, before product shutdown. It is recorded as a failed attempt, not a
successful fix. Normal relaunch restores the agent. Since the original UI still
inherited the opt-in environment, its replacement also starts tracing; a verified
safe-stop, closure of that known UI, and ordinary relaunch are then used to
remove the tracing environment. No service security policy, timeout or native
owner cleanup requirement is relaxed.

Untraced A run 542cc6d679c0cb72359437266453407f starts at 13:39:08Z. CLI connect
restores listening and both sides CONNECTED at 13:40:46Z. Collection
283f697f9f62000473ae160a287f5399 at 13:41:24Z confirms both protected services and
input READY; doctor is healthy and all builds match. That short post-recovery
interval has no new fault or trace-enabled event in the active A run. It is not
long-duration stability evidence. The user has been asked for three untraced
idle-to-entry round trips; ordinary grant/transit scheduling remains unresolved.
Do not change process priorities, GC policy or timeout budgets from the traced
confound. The fixed diagnostic regions are ready for later controlled tracing;
leave the flight recorder off during normal use and latency comparisons.
