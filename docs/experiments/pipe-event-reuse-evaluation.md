# Local input pipe event-reuse evaluation

Status: evaluated and rejected for the current latency work; no transport change
retained. Running devices remain on g355 and the pending g356 source still has
per-operation completion events. The separate exact-read cancellation change is
unchanged.

The Windows pipe transport creates one manual-reset event and one OVERLAPPED for
each read/write and closes the event after the operation completes. A local
candidate retained one event/OVERLAPPED per direction, serialized connect/read on
readMu, reset a slot only after completion/cancellation join, and disposed both
events after active.Wait in Close. This removed four allocations (128 bytes) per
32-byte request/response round trip. No authorization or injection was involved.

Initial sequential measurements on A showed the unmodified path at
326597/799614/273279 ns per round trip and reuse at 34539/32981/30244 ns. Those
separate runs appeared promising, but a same-harness comparison did not reproduce
the apparent latency improvement. In the paired harness, transient events took
40955/38420/56333 ns and reuse took 61496/51511/42564 ns. Each run used 1000 round
trips, repeated three times. The transient comparator disposed each completed
slot before the next operation, mimicking the original event/OVERLAPPED lifetime;
it was a test-only wrapper, not a separately compiled old revision.

The consistent evidence is reduced allocation/handle churn, not faster latency.
Round trips include scheduling of a locked worker OS thread, and this uncontrolled
desktop can change between runs. These numbers are not mouse-to-display latency,
do not explain the retained 79..104ms receiver tails, and do not demonstrate a GC
cause. A lower-allocation implementation alone is insufficient reason to add
event/OVERLAPPED reuse ownership to the current patch. The candidate and its
specialized tests/comparator were removed before commit or deployment.

Retain only BenchmarkNativePipeRoundTrip as a repeatable baseline transport probe.
It uses actual Windows named pipes with a locked worker thread, performs 32-byte
serialized request/response I/O, and reports time/allocations. It launches no
service, captures/injects no input and changes no live connection. Unit/native
correctness checks passed for the exploratory candidate, but they did not prove
its performance hypothesis. Existing native behavior remains the source of truth.

Next priority remains the g355 attended movement comparison and the separately
validated g356 preparation-watcher wakeup, rather than treating native event
creation as the established movement-stutter cause. No build, device update,
service restart, push, public release or CI run was made for this evaluation.
