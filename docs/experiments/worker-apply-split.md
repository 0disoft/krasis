# Bounded worker apply split (g178)

The g177 worst retained receiver input has WorkerApply 31ms within a 57.837ms
dispatch. WorkerApply currently includes input application and cursor readback.
The next measurement separates those two worker-local spans before any change
to authorization, cursor behavior, expiry or native input.

Optional `workerApply` in a handoff vector contains fixed `micros:[apply,snapshot]`,
two-bit `measured`, and two-bit `saturated`. Missing evidence differs from a
measured zero. Durations floor to microseconds and saturate at the existing
120-second bound, with explicit saturation bits. The spans describe the existing
apply function and subsequent cursor snapshot, not exclusively syscall time.
When the containing worker span is measured and unsaturated, the two parts must
fit below its next millisecond boundary; inconsistent numeric detail is dropped.
They are disjoint parts within WorkerApply, not additional end-to-end latency;
never sum them with their parent or subtract cross-process clocks.

The client requests detail only for actual installed receiver input samples in
the existing twelve-attempt/120-second window, at most 2,048 requests per
collector across reentry. Bootstrap, heartbeat and recovery cannot request it.
Only the original sequence/op-matching ACK attaches a copied profile; later
recovery cannot overwrite it. First/worst vectors keep their existing semantics.

The local request remains 48 bytes. KRI1 version 2 carries one diagnostic flag
in byte 7; version 1 remains readable and unchanged. Heartbeat/close reject the
flag. Detailed replies use KRA1 version 4, retaining the version 3 outer timing
layout in the first 24 bytes, followed by a 12-byte diagnostic block: two
big-endian uint32 durations, presence byte, saturation byte and two reserved zero
bytes. The private cursor suffix follows that block when applicable. Versions
1..3 remain readable. Installed service/agent images must match through the
existing update and image-validation lifecycle; old workers do not accept the
new request version. No peer-network protocol or authorization change.

A reply still uses one write and one roundtrip. Cursor replies read the detail
and private position suffix together, preserving their existing two reads.
Profiled non-cursor replies need an additional suffix read. This bounded overhead
is inside client response timing and must be considered in comparisons. Ordinary
traffic has no extension once the window/budget ends. Structurally incomplete or
reserved-byte-corrupt replies fail framing; invalid numeric detail is consumed
and dropped without losing cursor/next-frame alignment or changing valid ACK
status. Locally invalid/incompatible detail is omitted. No error text, operation
kind, input content, PIN, coordinates, identities or paths enter this diagnostic.

Hosted foundations cover probe budgets, immutable sample association, numeric
presence/saturation, partial framing, legacy replies, same-write cursor layout,
numeric rejection without outcome changes, and local/peer export sanitization.
## g178 native wiring

The client sets the flag after its existing successful authority check and only
records detail from a sequence/op-matching response. The worker creates the
profile inside the existing authorized desktop callback. Failed authority or
desktop scope produces no fabricated apply measurements. The original apply
function runs exactly once; snapshot follows only when required and when apply
succeeds. Post-move snapshot failure keeps outcome-unknown classification.
No input is replayed and cancellation/recovery follow their existing paths.

Hosted tests use virtual time to distinguish 31ms apply from a 2ms snapshot,
including failed/absent work, disabled profiling and snapshot-only behavior.
Client/worker transport checks cover flagged tracked input and unchanged idle
or untracked traffic. Apply g178 to both devices, repeat ordinary crossings and
PIN-open reentry, then export diagnostics to identify which measured part stalls.
This build narrows evidence; it does not claim reduced latency.

## g512: Same-PC native IPC phases

Within the existing bounded receiver probe, `workerApply.nativeIPC.micros`
contains six disjoint durations: parent executor admission/preparation,
request submission to child read/decode completion, child validation/admission,
child handler through reply encoding, reply-ready to parent's first positive
read return, and that return through matching response decode. These span one
exact sequence/op-matching exchange. Windows QPC provides a common monotonic
clock within one PC; no cross-laptop clocks are subtracted. Failed, zero,
unordered or over-120-second clock observations discard this metadata only.
The [Windows QPC guidance](https://learn.microsoft.com/en-us/windows/win32/sysinfo/acquiring-high-resolution-time-stamps)
also allows one counter tick of cross-thread ordering uncertainty. Durations
here are floored to microseconds, not a claim of sub-microsecond precision.

Request size stays 48 bytes; KRI1 v2 flag 7 requests apply, submission and IPC
detail together. Only the local portable/native client requests IPC; heartbeat,
close and release-only handlers cannot request it. KRA1 versions 15..28 mirror
1..14 with a trailing 32-byte block after all private cursor bytes. Mode 1
contains three child QPC microsecond stamps; mode 2 contains the six normalized
uint32 durations. Seven reserved bytes are zero. The journal emits only mode 2;
absolute counters never enter logs or peer support exports. Invalid optional
metadata is consumed and dropped; truncated framing still fails normally.
The ACK remains one write and one roundtrip, with 32 additional bytes only on
sampled exchanges. Unprofiled input and liveness have no extra clock calls.

Reply-ready is immediately before the existing Write call. The delivery span
includes write submission, kernel transport and parent scheduling; it does not
identify Write completion or kernel arrival. Child handler includes fresh
service/desktop checks and existing application work; nested SendInput and
worker timing fields remain separate, and must not be added to these spans.
No retry, new input, authority cache, deadline extension or cleanup exemption
is added. This is instrumentation; latency improvement requires fresh physical
evidence and a subsequent cause-specific fix.

### g513: Preserve parent stalls that exceed child apply

The g512 artifact was built but not installed. g513 corrects acceptance of the
same six IPC phases. Native IPC starts in the parent before executor admission
and is not nested inside child WorkerApply. A 1,116ms parent envelope with
184ms child apply is valid; acceptance must not depend on the child apply bound.
The IPC envelope remains independently limited to 120 seconds. A regression test
covers this real stall shape and preserves the separate child apply validation.

A proposed post-Write receipt was deliberately excluded: waiting for a child
scheduled after ACK Write would itself extend the input-critical path. The
existing one-write ACK framing and 32-byte sampled extension remain unchanged.
The reply-ready/first-read duration must not be described as pure write time:
separating actual Write completion from receive scheduling requires Windows OS
tracing after the six-phase measurements identify the affected path.
