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
Native wiring and its failure/cancellation tests are a separate bounded slice.
