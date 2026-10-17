# Matched slow dispatch evidence

Collection `efceda05bb32eb4f5e8299840ab3dc22`, B g329, contains a 197073us
dispatch maximum in the minute ending 2026-10-17T08:44:57.5215215Z. This is a
maximum across control, input and heartbeat dispatches, not necessarily an input.
The same minute contains a receiver handoff with AuthorizeTotal=192324us,
ConnectTotal=189291us, ReadyWait=146360us and PreparationNoCandidate. Its cold
startup includes ServiceChildConnect=106322us and WorkerInstalledValidation=
51120us. These spans overlap; do not add them. First input dispatch was 2210us
and all 63 subsequent input dispatches were below 10ms. This supports examining
cold entry admission, but does not identify the exact 197073us operation.

## One completed operation per observation

G330 retains the largest completed dispatch of each of five fixed message kinds
from a bounded queue in each runtime summary. Only dispatches >=50ms enter the
queue; faster work remains covered by existing summaries and handoff profiles.
Each value retains one operation's monotonic start offset, full duration,
read-completed to dispatch-start gap, explicit coordinator wait and
acquired-to-completed span (input/heartbeat only), and completed synchronous
injections' total, maximum and count. Unobserved phases have availability flags
or zero completed injection count. A control operation's internal locks are not
labelled as an observed coordinator wait. Entry profiling still owns its detailed
authorization, startup and worker phases.

The single serial session reader owns these callbacks and synchronous injection
callbacks. Samplers never read a multi-field in-flight operation as an exact
sample: completion publishes a scalar value to a nonblocking 32-slot channel.
This is memory bounded, callbacks allocate nothing, and queue loss increments
`slowDispatchDropped`. At most 32 values are drained at a summary to avoid
starvation by concurrent production. The summary retains at most five complete
values; it is the maximum of retained samples, not a full tail distribution.
An operation may start in a previous summary interval. Do not combine a span's
injection maximum with another operation or with an independent interval maximum.

Completion includes errors and cancellation; it does not mean input succeeded.
Compare session failure records and allowlisted handoff outcomes. Unfinished
operations remain represented by existing dispatch/injection ages and failure
trails. Offsets are local to the agent runner, not cross-PC clocks. This does
not measure TLS/network time before ReadFrame completed, OS cursor presentation,
or the source-to-visible end-to-end experience. Existing failure GC trails are
still failure-triggered; this adds no success-triggered GC trace.

## Verification and operation

Tests separate a contended semantic gate from injection, preserve a 200ms
dispatch's own 2ms injection when another operation injects for 80ms, distinguish
control/input/heartbeat, retain unknown acquisition on cancellation, exercise
queue loss, verify zero callback allocation, and round-trip sanitized export.
Even maximum numeric fields in all five spans plus full record identity remain
inside the existing 4096-byte runtime record limit. Unknown fields are removed
and malformed measurements rejected. Legacy summaries without spans remain
valid. No text, key, pointer coordinate, PIN or window data is collected.

This change narrows the next diagnosis; it does not tune GC, extend prepared
child lifetime, increase lease/timeouts, or replay input. No public API, peer
wire, DB, dependency, ACL or runner change. Rolling back uses the ordinary peer
update mechanism; no persistent setting or data migration is required.
