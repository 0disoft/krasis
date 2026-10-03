# Bounded handoff profile

`handoff_profile` schema 1 measures software processing within each ordinary
agent process. It does not measure pointer presentation, establish authority,
prove recovery completion, or change the existing error certainty. Wire frames,
worker ACK formats, worker lifetime, authority checks and the service accept
backoff are unchanged. g139 extends only the authenticated local readiness
envelope as described below; peer frames and lease metadata remain unchanged.

The product runner shares one collector across reconnects. The first attempted
handoff opens a 120-second window, with at most 12 attempts per agent process.
Failed requests consume slots. A return or rejection emits the attempt once;
session exit or expiry emits unfinished attempts as incomplete. There is no
reuse of the last slot after the attempt budget is exhausted. Starting an
untracked handoff freezes any old profile for that role as incomplete. Capture
activation aborts and matching inbound aborts finish with diagnostic `error`,
even when the control runtime successfully recovers and returns no error.
There is no per-input disk output. Summaries use the existing bounded diagnostic queue.
Queue loss remains visible through the logger's existing dropped/failed state;
missing records are not evidence of a successful or zero-duration operation.

Each summary has a local attempt ordinal, source/receiver role, local receiver
backend (`native` or `installed`; source is `none`), preparation vector, first
attempted actual frame vector, worst steady-state frame vector, five-bin
steady-state frame histogram, end result and incompleteness flag. `frames`
excludes the first frame and counts measured frames, not physical mouse events.
The first failure remains the first sample. Heartbeats do not reserve it.
Recovery calls within a dispatch do not overwrite the initial client timings.
The worst vector belongs to the frame with the longest whole dispatch; it is
not assembled from independent stage maxima.

`micros` is a fixed array indexed by `internal/handoff/profile.go`'s `Stage`
constants. `measured` is its presence bitmask: an absent bit means unobserved,
even if the value is zero. Durations saturate at 120 seconds. Histogram and
worker bucket boundaries are <1ms, <10ms, <100ms, <1s, >=1s. Worker bucket zero
means unobserved; the three slots are authority, scope and apply from the
existing validated matching ACK. They overlap client response wait and cannot
be added to it.

Preparation includes boundary decision to begin (queue **and admission**),
request coordinator and write, request to grant, peer authorization and its
full wrapper, installed open lock/connect, pipe open, service authentication
and its image pin, lease write/ready wait/new-client validation, return arm,
entry prepare, runtime prepare, grant write, grant coordinator, source and
capture activation, commit write and receive-side commit coordinator/apply.
Input vectors include source queue residence, first offer/dispatch since the
local attempt began, input coordinator/write, complete send/receive dispatch,
outer installed-input lock/client call, existing client timing stages, validated
worker buckets and cursor/return postprocessing. Preparation also records
source deactivation, installed channel close and the gap after the previous
local close/return. A channel-close return is **not** a worker recovery receipt.
Ready wait includes launch, child authentication and desktop preparation; it
does not isolate CreateProcess time. Native receiver peer authorization is
included in the full wrapper; its narrower installed-only span is absent.

Not measured separately: rejected local admission before a lease request,
service cleanup/200ms accept backoff, lower-level process creation substeps, OS cursor-restore
message handling, actual display presentation. Missing spans must not be filled
from older 15-second maxima or assumed to be zero. No stage maxima, overlapping
spans or opposite-PC wall clocks may be summed into end-to-end latency.

## g139 installed startup detail

An installed receiver profile can additionally contain `startup`, schema 1,
with a fixed eight-element `micros` array and eight-bit `measured` mask.
The original 39-stage vectors, handoff schema, 120-second window and
12-attempt budget are unchanged. Missing `startup` or absent presence bits
mean unobserved. A numeric zero with its presence bit is measured at the
recorded precision. All startup durations saturate at 120 seconds.

The indices are owned by `internal/handoff/startup.go`:

| Index | Span | Measurement boundary |
|---|---|---|
| 0 | ServiceLaunch | `launchManagedConsole`, including privileges, validation, token, image pin, process creation and transferred handle setup |
| 1 | ServiceChildConnect | Service's wait for the child's control-pipe connection |
| 2 | ServiceBootstrapReady | Bootstrap packet write through receipt of child readiness |
| 3 | WorkerInstalledValidation | Child's installation digests and fixed SYSTEM/UIAccess image validation |
| 4 | WorkerIdentity | Child's process/token validation in `runConsoleInput` |
| 5 | WorkerServiceAuth | Child authenticating the service control pipe |
| 6 | WorkerAgentAuth | Child authenticating the transferred agent pipe |
| 7 | WorkerDesktopReady | `inputdesktop.Run` through first desktop-scope and authority validation, before invoking readiness |

These are durations from each process's monotonic clock, never cross-process
timestamps. Work in the child overlaps service waits; do not sum the rows.
Process initialization before these boundaries and later desktop switches
remain unmeasured. Failed startup has no complete child readiness detail.

Only readiness uses a fixed 40-byte `KRD2` local envelope instead of the
legacy eight-byte `KRD1`. New readers accept either; legacy readiness leaves
detail unobserved. The child can contribute only worker spans, never replace
service-local timings. Existing process/image authentication and verified
desktop preparation still precede readiness. Input requests, ACKs, recovery
receipts, deadlines and authority checks are unchanged.

Malformed or truncated readiness still fails the existing protocol path.
Invalid numeric diagnostics in a structurally valid receipt are consumed and
discarded without changing readiness. No diagnostic value grants authority.
Only validated detail is copied once into the active installed receiver
attempt; late or duplicate detail cannot replace a frozen attempt. Export
reprojects this fixed numeric schema and drops arbitrary fields.

Deploy the matching g139 protected installation before measuring this detail;
updating the portable UI alone does not update the SYSTEM service. Old readers
do not accept KRD2, so do not run mixed service/worker/agent binaries. Keep the
existing verified installation, image matching and recovery gates in place.
Record actual protected backend selection before the six-entry trial, then
recheck PIN Esc and cancel click without entering any PIN value.

No peer identifiers, lease/binding values, paths, SID, coordinates, operation
kinds, key values, window text or error strings enter the profile. Session and
epoch are used only in memory to reject stale diagnostic association. The log
envelope retains build/run metadata but omits the connection identifier for
this event; export validates and reprojects the fixed schema.

## Paired device trial

1. Apply the profile build to both agents and confirm each selected receiver
   backend from the new diagnostics. Do not assume both use installed input.
2. Within two minutes of the first crossing, use A's physical mouse to enter B
   three times: initial entry, quick return/reentry, then return and wait at
   least one second before reentry. Keep moving briefly after each crossing.
3. Repeat using B's physical touchpad as the source for three B-to-A entries.
   Returning A's mouse to A retires its lease; it is not a new B-source sample.
4. If ordinary input is normal, separately check A's Esc and cancel click in
   B's PIN dialog. Do not enter, capture or log any PIN value.
5. Export both diagnostics. Match only known trial order, role and completion;
   ambiguous or missing pairing stays incomplete. On error, stop the trial;
   never replay uncertain input or change security policy for this experiment.

This build is instrumentation only. The next performance change must follow
the measured bottleneck. Revert this commit if profiling interferes with input,
recovery, handle lifetime or diagnostic privacy.

## g145 worker timing detail

`handoff.Vector` gains additive fields `workerMillis` (fixed `[3]uint16`) and
`workerMeasured` (three-bit presence mask) with slot order authority, scope and
apply, matching the existing validated matching ACK. The existing worker bucket
array, the client response microseconds and profile schema 1 are unchanged.

The local `KRA1` ACK is still exactly 24 bytes on the same read/write path, with
no additional calls or round trips; the existing `io.ReadFull` still accepts
fragmented reads. Version 3 reuses bytes 20..23 as a
big-endian `uint32` holding three ten-bit fields, slot `i` in bits
`i*10 .. i*10+9`, with bits 30 and 31 zero. Encoded 0 is unobserved, 1 is a
measured zero and 1023 is saturated at `MaxWorkerMillis` (1022 ms). A present
slot decodes to `encoded-1` milliseconds, and `workerMeasured` derives from the
present slots. Encoded 1023 is a lower bound of at least 1022 ms. For floored
milliseconds this reproduces the legacy bucket exactly: values in 1000..1021 ms
are not saturated and keep their own bucket, every span of one second or more
stays in the legacy `>=1s` bucket, and only values at or above 1022 ms saturate.
The writer emits version 3 only when precise detail is present; the
decoder keeps the existing version 1 and 2 behavior, and a new client still
accepts the legacy 24-byte ACK with the detail unobserved.

Milliseconds are floored, so sub-millisecond spans truncate to zero. Validation
rejects a value above 1022, a nonzero value without its presence bit, and a value
that disagrees with its bucket. An absent bit stays unobserved even when the
value is zero, so a measured zero is distinct from a missing sample. Values come
from the same sequence- and operation-matching ACK and are retained on the first
and worst dispatch vectors. Unrelated dispatch maxima must not be combined, and
overlapping spans must not be subtracted into a claim about OS scheduler time.

No value outside the ten-bit fields is representable, so no numeric detail is
dropped. Valid `KRA1` ACK statuses and authorization are unchanged; malformed
reserved bits, an all-zero detail word, or a truncated or structurally invalid
ACK retain the existing protocol-failure and unknown-outcome handling with no
retry. Nothing added here records input content, operation kinds, coordinates,
paths, SIDs or PIN values, and there is no new per-input log; the existing
120-second window and 12-attempt bound still apply. Service, agent and worker
images must match with no permission weakening.

This is instrumentation only, not a confirmed stutter fix.
