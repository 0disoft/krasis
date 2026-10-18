# Bounded handoff profile

## g319 later handoff windows

The product runner uses `handoff.NewRolling`: at most 12 attempts in each
120-second window. Expiry freezes and emits unfinished attempts. A later real
handoff starts the next window; reconnects and rejected attempts within a window
cannot replenish its budget. There is no periodic restart or background polling.
The single-window `handoff.New` contract remains available for one-shot trials.

Old timers and retained callbacks cannot expire or modify a later window. Attempt
ordinals restart at 1; pair exported records only by their connection-local
`exchangeTag`, never by ordinal. Native cycle, worker-apply and cursor-placement
probe counters keep their collector-lifetime limits across all windows. No input
content or additional wire fields are collected. This makes later trials visible;
it does not establish a latency improvement.

## g182 optional per-exchange diagnostic tag

`Summary` gains optional `exchangeTag`, exactly 64 lowercase hexadecimal
characters. The authenticated transport derives it from a dedicated TLS exporter
label `krasis/handoff-diagnostic-correlation/v1` and a SHA-256 context of
length-prefixed session/epoch identifiers. It returns only the SHA-256 digest of
32 exporter bytes and clears those bytes. Raw identifiers, exporter material,
timestamps, input content and user/desktop identities are not exported.

The same request has the same tag on its two peers, independently of their local
attempt ordinals. A different session, epoch or TLS connection has a different
tag. This is a connection-scoped diagnostic label, never an authentication token
or replay, lease, input or recovery decision. There is no new wire field, read,
write, handshake or roundtrip. The supervisor forwards the optional capability
and each `RunSession` binds its current connection's mapper before reader startup.

Derivation runs once per retained source or receiver attempt, outside the collector
mutex, under the existing 12-attempt/120-second budget. Expired/untracked attempts
do not derive a tag; late results cannot update a retired attempt. Missing or
invalid derivation yields no field and cannot change input outcome. Derivation
cost is included in existing preparation/reader spans; its device overhead is not
independently measured. Older profiles with no field remain valid. Local and peer
export validate the fixed tag shape and omit unknown/raw identity fields.

For a paired-device export, join only one source and one receiver with the same
exact build and nonempty tag. Report duplicates, missing counterparts, incomplete
profiles and absent/saturated durations separately; do not match by ordinal or
nearby wall timestamps. Compare A's grant wait and B's handling for that request
as independent host-local intervals. Write completion/read boundaries can overlap;
do not subtract their timestamps or durations to call the result network RTT,
packet loss or scheduler cost, and do not add nested preparation/input spans.
Tags cannot make forged diagnostics authoritative. Device validation must first
confirm unique peer matches and then inspect slow matched exchanges.

## g163 optional preparation-lock overlap

Schema 1 gains optional preparationLockMicros (uint32) and preparationLockMeasured
(bool). These represent the intersection of receiver OpenLock and the last completed
local preparation interval. The resident agent samples the start immediately after
acquiring input.mu and retains it only if preparation passes its guards. It samples
the end after the acknowledged/missed callback, immediately before unlock. OpenLock
starts before lock acquisition and ends after it; only the clipped overlap is exported.

The recorder requires a receiver attempt and a measured OpenLock, preserves its first
observation, saturates elapsed values at 120 seconds and ignores overlaps exceeding
the measured OpenLock. Export validation rejects impossible bounds, wrong roles and
nonzero values with an absent flag. The 12-attempt/120-second collector still applies.
A present zero means no measured overlap with the latest interval, not proof of no
other lock wait. Retaining only the latest interval makes this a lower bound rather
than a complete ownership history. Do not add it to OpenLock or infer sole causation.
No timestamps, PIN values, paths or identifiers are exported. Peer frames, ACKs and
the 39-stage vectors are unchanged; older profiles without this detail remain valid.

## g162 additive exchange detail

`Summary` gains optional `exchange`, schema 1, with a fixed `micros [4]uint32`
array and a `uint8` `measured` mask. The indices are owned by
`internal/handoff/exchange.go`:

| Index | Stage | Measurement boundary |
| --- | --- | --- |
| 0 | RequestWrittenToGrantRead | Source: successful local request write completion to the full grant frame read |
| 1 | GrantReadToHandler | Source: full grant read to `Granted`, after grant validation/`Matching` and before the coordinator |
| 2 | RequestReadToHandler | Receiver: full request read to request handling, after the request checks and `Begin` |
| 3 | RequestReadToGrantWritten | Receiver: full request read to successful local grant write completion |

A source may set bits 0 and 1; a receiver may set bits 2 and 3. Unknown bits, a
zero mask, values above 120 seconds and nonzero values without a presence bit are
invalid. An absent bit is unobserved even if its value is zero. A present zero
means an observed span floored to zero microseconds. Export reprojects the fixed
numeric schema and drops arbitrary fields; old profiles without `exchange` remain
valid.

All endpoints use process-local `time.Now`, independently of the policy's injected
`ObservationClock`. The read timestamp is captured immediately after a complete
control frame is read, before progress observers, the policy clock and dispatch.
Only successful writes supply write completion. Source endpoints are remembered
once per attempt so reversed callback ordering can still produce a valid span.
If the grant was actually read before the local write returned, that overlap is
unmeasured. Negative/reversed spans are omitted rather than reported as zero;
elapsed values saturate at the existing 120-second limit.

The source wait includes peer work, network and local read-loop scheduling; it
does not isolate network RTT. Receiver stages share one read origin and overlap;
do not sum them or subtract wall clocks across devices. The 39-stage vectors,
peer frames/payloads, worker ACK, PIN authority and lease lifetimes are unchanged.
The existing 12-attempt/120-second collector still bounds emission. No input
content, PIN values or peer/lease identifiers are exported, and no extra event
stream is introduced. This is diagnostic instrumentation; device latency and PIN
regression must still be checked on the matching build.

## Existing profile contract

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

## g146 installation pin reuse detail

The g145 trial on receiver B gave 11 complete profiles and 924 further frames
(histogram 53 `<1ms`, 871 `<10ms`, none `>=10ms`, worst dispatch 4.062 ms). First
worker apply was 2..3 ms and worker authority/scope a floored 0 ms (under 1 ms,
not missing); the historical 198 ms did not recur. Boundary preparation spanned
223.349..265.820 ms, with service image pin 27.232..40.153 ms and service child
connection 153.823..183.777 ms. One export log is incomplete: the B export contains
11 receiver profiles and A contains 12 source profiles, the snapshot completion
differs, and the final receiver profile is unobserved in the supplied export. Do
not infer an end time or exact cause. All samples are A-to-B from source A, so there
is no new paired B-source evidence.

g146 makes the product connection own its verified installation pins via
`OpenAgentService`, reusing a duplicate of the same file object in each service
image authentication without rehashing the verified self image at selection.
Kernel pipe owner PID, SCM binding/configuration/status, image ACL, and
installed-agent token validation remain checked, and nil paths leave legacy behavior
unchanged. A boundary return closes only its lease;
product shutdown closes the client heartbeat before `installation.Close`, with no
stale pin or late open. Sharing keeps the existing write/delete exclusion and is
not a pathname- or digest-only cache. The goal is to remove the repeated 27..40 ms
hash work; child startup stays unresolved and the 198 ms tail unknown. This is not
a measured g146 speed claim and adds no private input logging.

## g146 observed device trial

Both exports come from build c5fde05 (A created 2026-10-03T18:11:16Z, B created
18:11:14Z). A holds 11 source profiles and B holds 10 receiver profiles, all with
source A, so there are still no physical B-source measurements. B kept 2082
continuous frames with histogram 255 `<1ms`, 1820 `<10ms`, 7 `<100ms`, 0 `<1s`,
0 `>=1s`. The worst B dispatch was 16.073 ms with the same ACK workers `[0,0,12]`
ms; two others reached 14.975 ms (`[0,0,14]`) and 13.598 ms (`[0,0,12]`). The
historical 198 ms did not recur; this is not a complete-fix or p95 claim.

| Span | g145 | g146 |
|---|---|---|
| Service image pin | 27.232..40.153 ms | 0..615 us, present in all 10 |
| Service authentication | ~29..41 ms | 1.771..2.847 ms |
| Receiver AuthorizeTotal | 223.349..265.820 ms | 177.842..208.960 ms |
| Child connection | 153.823..183.777 ms | 139.156..163.956 ms |

Attempts 8 and 9 failed with `activation_capture_foreground_missing_before_show`
(18:10:57.9690119Z, 18:10:59.7108724Z). On B the same ordinal reached startup
ready but produced no first frame and ended with an error. Both agent logs show one
`CONNECTED` with no disconnect, reconnect or backend failure after it. Missing
source foreground caused the activation rejection; why `GetForegroundWindow`
returned NULL is unconfirmed, so do not blame Windows or the application, change
the guard, or bypass the focus checks.

The final receiver profile in the B export is absent and stays unobserved; do not
infer cross-clock causality. Record only observed findings and keep the current
gate distinct from an unconfirmed root cause.

## g147 stateless injection port (static inspection)

Static inspection found the SYSTEM worker built a full `wininput.NewInputAdapter`
even though the worker adapter never calls `ArmReturn`. Its `Move` ran `SendInput`
then `GetCursorPos`, two `GetSystemMetrics` calls and a state-mutex return, while
the residentagent delegated outer adapter already owns and performs the real
cursor and return tracking. g147 extracts `wininput.NewInjectionPort` returning a
stateless `application.InputPort` with native `SendInput` only: `Press`/`Release`
reuse the shared `writeNativeDiscrete` validation, and `Move`/`Wheel` reuse the
same pointer/wheel encoding and `sendInputs` path. Native complete, zero and
partial counts and cancellation are unchanged, with no retry or fallback. The
worker uses it only after the existing authority, desktop and lease gates, and
worker `control.PressedState` still owns recovery. Outer residentagent entry and
return tracking is unchanged.

This only removes static duplicate work. It is not claimed to cause the 12..14 ms
tails or to fix the NULL foreground result, and hardware performance and PIN
verification remain pending.

## g148 concurrent fixed-image verification

`loadInstallationDirectory` now verifies its two fixed executable images
(`krasis-agent.exe`, `krasis-input.exe`) concurrently with a fixed fan-out of
exactly two goroutines, never a caller-sized one. Each goroutine runs the
existing hash-and-pin verification; the directory load joins both before
returning, owns every successful pin, and then runs the existing protected
handle ACL check. If either verification fails, both goroutines are joined and
every pin is closed, so one partial image never escapes; the caller still gets
`ErrAuthority`. No digest cache, pathname-only trust or authentication
relaxation is added, and the per-lease pin reuse from g146 is unchanged.

`Installation.loadElapsed` is a new field set once at the end of the load and
not mutated afterward. `service_windows.go` now records
`WorkerInstalledValidation` (startup slot 3) as `installation.loadElapsed +
time.Since(started)`, so the slot also covers the load and hashing that
previously ran before the function was entered. The startup wire format, schema
1 and the eight-element `micros` array are unchanged. This span overlaps the
service's `ServiceChildConnect` wait and must not be summed with it. Because the
measurement boundary changed, g148 slot 3 is not directly comparable to a g147
slot 3.

A local microbenchmark of the file verification alone on Windows Ryzen 5 7430U,
with two 8 MiB temporary files and `benchtime=300ms`, `count=2`, measured the two
sequential verifications at 32.599 and 35.450 ms and the two-goroutine path at
13.589 and 14.569 ms. This is a file-verification microbenchmark only: the real
B installation, PIN and desktop-switch total latency remains unverified, and
these numbers are not an end-to-end handoff claim.

## g149 client authority-query detail

`handoff.Vector` gains `authorityMicros [3]uint32` and `authorityMeasured uint8`.
The three client-local subspans of the existing `ClientAuthority` stage are
index 0 the authority mutex wait, index 1 `GetNamedPipeServerProcessId` and
index 2 `QueryServiceStatusEx`. Values are elapsed microseconds clamped by the
existing 120-second window; the low three bits of `authorityMeasured` each mark
one observed index. A missing bit is unobserved even at value zero, and a
present bit with a floored zero means observed under one microsecond. These are
not new wire or authority fields: the 39-element `micros` array, the eight-element
startup array and the 24-byte `KRA1` ACK are unchanged.

Only the actual client input dispatch context records these subspans; heartbeats,
startup and recovery do not, and recovery cannot replace a first measured call.
The detail attaches to the same first and worst dispatch vectors as the existing
worker and input fields, and unrelated dispatch maxima must not be combined.

The observed authority wrapper runs exactly the same mandatory validations as
the plain path, with no cache, skipped check, new trust or input retry. A failed
query still records the elapsed span it spent before failing, and an incomplete
substage set is absence, never success. This is instrumentation only: it is not
claimed to fix the stutter, and PIN and hardware results remain pending.

## g150 source capture activation timing

`Summary.Capture` is an optional `*CaptureProfile` (schema 1): a
`micros [21]uint32` array and a `measured uint32` presence mask whose low 21 bits are used. The indices are
`CaptureSerializeWait`, `CaptureAdmission`, `CapturePost`, `CaptureQueueToHandle`,
`CapturePosition`, `CaptureTouchpadRegister`, `CaptureShow`, `CaptureUpdate`,
`CaptureForeground`, `CaptureFallback`, `CaptureActiveCommit`,
`CaptureHandlerTotal`, `CaptureResultWait`, `CaptureRollback`, `CaptureTotal`, `CaptureBoundaryBefore` and `CaptureBoundaryAfter`
(0..16), followed by the installed-source IPC lock, authority, write and response
spans (17..20). Values are elapsed microseconds clamped by the existing 120-second
window; an absent bit is unobserved, and a present zero is a clock-reported zero,
not proof that no wait happened.

Recording is scoped to the matching source attempt for `ActivateProfile(ctx)`,
whose context is carried across the local capture thread. `RecordCapture` takes
the collector lock and drops a finished, expired, wrong-role or duplicate sample,
so a late timeout or completion cannot contaminate a newer attempt. The legacy
`Activate()` and the existing source interface still work.

These spans nest and must not be summed: `CaptureQueueToHandle` runs from
admission start to the handler, so it overlaps `CaptureAdmission`/`CapturePost`;
`CaptureResultWait` overlaps the remaining handler work and response delivery; `CaptureForeground` contains the request
and verification, `CaptureFallback` is separate, and `CaptureTotal` is the underlying capture command's caller duration, overlapping with owner work; a late owner span is dropped once the original attempt is retired. The wrapper guard lookups (the two boundary stages) are part of the overall `CaptureActivate` span, not of `CaptureTotal`.
The 39-stage vectors, the `KRA1` ACK and the startup schema are unchanged. Capture
numerics are validated on the local schema and reprojected by the existing remote
sanitization. No coordinates, window text, input, PID or paths are exported. This
is instrumentation only, not a stutter-fix claim.

g340 also projects the existing activation ACK's `worker [3]byte` into this
same capture attempt. Index 0 remains unobserved; indices 1 and 2 are the command
desktop scope/authorization and native apply buckets, respectively: 0 unobserved,
1 below 1ms, 2 below 10ms, 3 below 100ms, 4 below 1s, 5 at least 1s. Scope excludes
the nested native apply duration. The containing IPC response also includes
queueing and delivery, so these buckets cannot be subtracted as exact times.
Detail is accepted only after the full matching activation ACK, including a
known rejection, and requires a measured CaptureIPCResponse span. A malformed
or foreign ACK contributes no worker detail. The first recorded ACK is retained;
retired and expired attempts reject late updates. There is no extra IPC, new
wire field, change to authority checks, or claim of lower capture latency.

## g152 worker installation load detail

`StartupProfile` keeps schema 1, the eight-element `micros` array and the
`measured` mask, and adds optional `loadMicros` (uint32) and `loadMeasured`
(bool) on schema 1 — optional fields, not a ninth element of the `micros`
array. `loadMicros` is the file/metadata load duration inside
`WorkerInstalledValidation`: `RunInstalledWorker` in
`internal/inputworker/service_windows.go` records the containing total first and
then calls `RecordLoad(installation.loadElapsed)`. It is a subspan of that
stage, not an additional contribution to startup elapsed and not an authority
signal. An absent `loadMeasured` flag means unobserved; a present flag at zero
is a clock-reported zero. `Valid()` rejects an unmeasured nonzero load, a load
above the 120-second window, a measured load without the containing
`WorkerInstalledValidation` bit, and a load greater than the containing total.
`mergeWorkerStartup` copies child load detail only into the child-owned spans
and never replaces a service-owned measurement.

A readiness receipt that carries load detail is `KRD3`: 48 bytes, the existing
`KRD2` 40-byte envelope plus an 8-byte extension (4-byte big-endian load
microseconds, 1-byte presence, 3 reserved zero bytes). The writer emits `KRD3`
only when `loadMeasured` is true, otherwise keeps `KRD2`, and a profile that is
invalid or has no measured stage still falls back to the legacy `KRD1` frame.
The reader accepts `KRD1`, `KRD2` and `KRD3`; a truncated envelope, an unknown
header or nonzero reserved bytes fail with the existing protocol error, while a
structurally valid but numerically invalid payload is consumed and dropped
without changing readiness. An older reader does not accept `KRD3`, so the
cryptographic product installation must match before measuring. Peer frames, the
`KRA1` ACK, image matching and authority checks are unchanged. This is
instrumentation only, not a latency claim.

Separately, g152 binds the desktop object name `Default` to the exact owned
handle once at acquisition and still queries UOI_IO before every work; a replaced
handle does not inherit the previous name proof. That is not a profile change and
is measured in `docs/engineering/03-performance-budget.md`.

## g320 installed cursor-entry vector

`Summary.entry` is one optional receiver/installed `Vector` for the cursor entry
transaction, separate from `preparation`, `first`, `worst` and the frame histogram.
Its `Dispatch` duration covers the entire `PrepareEntry` call; `ClientResponse`
and the matching ACK's worker authority, desktop and apply times are nested spans.
They must not be added together or subtracted as a proven scheduler residual.
The g319 `preparation` vector retained the first client command timing without
the matching worker ACK details; it cannot isolate where its 1.94s wait occurred.

The new batched transaction has one input IPC command. The ordinary matching
worker-detail/submission probe budget also bounds this one entry sample per
tracked attempt. Entry never consumes the first input, increments frames, or
changes authority, request deadlines or uncertain-input policy. Completion is
recorded once; retired/expired attempts drop late completions. An absent entry
means unobserved, not zero latency. Numeric validation and the existing remote
export projection strip arbitrary properties; no coordinates or input values
are added to support files. Device latency remains to be measured.

## g321 ACK read progress

Installed receiver vectors (`entry`, actual `first` and `worst`) may include
`ackRead`: `micros [3]uint32`, three-bit `measured`/`saturated` masks and numeric
`progress`. The three disjoint client-local spans are: read start to the first
existing `Read` returning positive bytes; that return to completion of base
24-byte header reading and decoding; header completion to suffix reading and
decoding. They are inside `ClientResponse`, not additions to it. The first span
measures the client's observation, not kernel arrival or a proven scheduler wait.

Progress is 0 (no positive read), 1 (data returned, header not decoded), 2
(header decoded, remainder not decoded), 3 (complete response decoded), or 4
(sequence/op matched). A matching error ACK is also 4; the existing outcome
records its failure. Failed reads retain the active span through error return;
unentered stages have no measured bit. Partial/foreign ACKs retain client-only
read progress, never matching worker detail or cursor coordinates. A positive
Read with an error keeps the underlying `io.ReadFull` completion semantics.

Only the existing selected `command.applyDetail` enables observation; it never
calls `WantWorkerApply` twice. The wrapper delegates identical read sizes,
counts, bytes and errors, without a probe read, goroutine, deadline change or
wire extension. Unselected requests/heartbeats keep the original path. Metadata
is copied once into its original sample and validated against presence, fixed
progress, saturation and the containing unsaturated `ClientResponse`. Missing
profiles are unobserved, never zero. No input values, coordinates or OS names
are collected. This change locates stalls; it does not itself reduce latency.
