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
