# g522: idle preparation and resident scheduling pressure

Baseline: g521 paired collection `c6f0d2ece711fe0b0631c17c3e28a667`, recorded
in `2026-11-03-g521-paired-crossing-review.json`.

## Receiver preparation

B's worst preparation was 219.995ms, with a console candidate only 5.297ms old.
Native desktop readiness took 135.150ms; native children recorded roughly
75-80ms each hashing their images. These spans overlap and must not be summed.
The old 15-second spare expires during ordinary multi-minute use of the other
PC. g522 retains the same unbound reservation for a fixed 15 minutes, moving
construction out of subsequent entry when that existing candidate survives.
See `prepared-worker-selection.md` for the unchanged binding/retirement checks
and the longer memory/image-pin residency tradeoff. Cold entry and entry after
the new limit can still incur construction; this is not a measured improvement.

## Source scheduling pressure and attribution

A's 1852.582ms maximum came from a diagnostic timer aggregate, not a measured
same-duration input or heartbeat wait. Retained OS samples show saturated system
CPU with little Krasis process CPU. A separate 1056ms late interval includes a
GC pause in the >=1s bucket; there is no timestamp-matched proof that GC caused
the full 1852.582ms interval. OS observation reads also reached 210.795ms.

The resident alone now defaults to GOGC=300 rather than the runtime's default
100, reducing repeated GC stop requests for its small live heap under host CPU
pressure. Native/capture/guard children retain their existing policy. Explicit
GOGC is preserved, including 0/off/empty; GOMEMLIMIT is not changed. The tradeoff
is a larger heap, not disabled GC. This cannot fix competing host CPU load or
guarantee the disappearance of scheduler stalls.

A fixed-allocation synthetic benchmark on this laptop (2MiB retained heap,
32768 allocations of 4096 bytes = 128MiB) with the packaged runtime overlay
recorded 26 GC cycles at 100 and 12 at 300: 53.8% fewer cycles. Final heap was
7.755MiB and 14.01MiB respectively. An earlier stock-runtime run recorded
32 versus 11 cycles; counts depend on heap/runtime state and host scheduling.
This measures GC pacing only, not physical input latency, peak memory, or the
cause of the original stall. Benchmarks use real runtime counters; tests also
verify explicit operator overrides and memory limits remain intact.

Runtime summaries now separate `timerLateMicros` (liveness timer observations)
from optional `samplerTimerLateMicros` (GC/minute diagnostic timer observations).
Long stalls in either still request existing bounded trace capture. Histograms,
OS timing, nonfatal observations and fault evidence remain available. This is
an additive diagnostic JSON field, not a control/input protocol change. No
heartbeat timeout, lease authority, replay behavior or reconnect policy changes.

## Verification boundary

Virtual-clock preparation tests and agent/runtime/log/API tests pass locally.
Physical crossing latency and recovery recurrence require the post-install
trial. Historical timer maxima cannot be compared directly to the new liveness
field; compare old maxima to `samplerTimerLateMicros` and check validated
heartbeat progress separately. No remote CI or push is part of this update.
