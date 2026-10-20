# Portable input process boundary (#50): transport ready, product wiring pending

## Current authoritative state

The product and cmd/krasis-agent routes are unchanged from the verified g388
implementation. AgentGeneration remains 388. No new package or application is
created in this round; installed A/B continue e1954c28f5edb74e654104e43b9bee8d6e4f9ac6.
Doctor remains healthy/CONNECTED with matched protected UI/agent/service images.
The ordinary agent executable intentionally still rejects the staged portable
child argument. There is no new listener, SYSTEM authority or automatic fallback.

The staged internal transport adds a fixed ordinary-child launcher and an
ExchangeInput operation to the g388-owned Process lifetime. Each exchange shares
the serial gate with local lifecycle IPC and honors the smaller caller/two-second
budget. Only the fixed input launcher enables the exchange; an ordinary crash
guard or zero Process cannot impersonate it. The production routes do not call
that launcher yet.

NewPortableClient reuses Client's input/cursor validation and response decoding
without creating any Channel or Authority. It carries input, cursor snapshot,
single placement/readback, entry snapshot/placement and fixed typed certainty.
The child dispatcher validates binding, sequence, lifecycle and supported frame
shape before invoking the supplied desktop executor. A native failure closes
further input admission but keeps guard control frames available. No request is
replayed, and a different backend never receives a lost/unconfirmed operation.

An atomic submission state distinguishes unstarted, attempted and permanently
fenced callbacks. If timeout fences an unstarted callback first, it can never
later write input. A callback that wins admission before timeout retains unknown
certainty after any missing ACK, including zero-byte stdio failure. Late callback
data remains owned by the original operation and cannot overwrite client state.

## Validation completed

Inputworker, crashguard and residentagent suites passed; cmd/krasis-agent compiles
in its test command. Vet passed for the three modified runtime packages. No
race detector is available (CGO/compiler unavailable); no hosted CI was invoked.

Real hidden child fixtures exercise inherited stdio and actual process retirement
without performing native input. Tests cover positioned move, cursor get/set,
placement and entry snapshot, discrete press/release, horizontal/vertical wheel,
interleaved guard control, ordinary guard rejection, canceled admission, partial
and not-applied failure ACKs, journal cancellation after known non-application,
lost ACK after simulated application, non-cooperative operation timeout and no
replay. Unit cases additionally reject replay, changed/zero binding and invalid
close frames before input, and prove pre-dispatch desktop rejection is not-applied
and permanently fences further input dispatch. Native desktop/physical input,
parent-death held-input recovery and product wiring are NOT validated by these
fixtures.

## Why activation is deliberately incomplete

An initial integration placed ordinary native input and the journal in one child.
That is not acceptable as the final design: if SendInput/readback blocks while
the resident parent dies, the same blocked child cannot read EOF and release its
journal. Killing that child also kills the only fail-safe copy. The existing
independent crashguard behavior must survive the process split. That integration
was removed before commit; no user runtime received it. The staged dispatcher is
a transport component, not evidence that #50 or its physical fault gates passed.

## Exact continuation scope

1. Keep a distinct ordinary input child I and independent recovery guardian G
   under resident parent R. R sends journal prepare/cancel/released to G and
   input/cursor requests to I; serialized IPC must preserve current action order.
2. Bind G to exact pinned I and R kernel process lifetimes through a closed
   inherited-handle bootstrap. Do not accept a peer-selected PID, executable,
   desktop, token, address or service identity. Keep ordinary-user permissions;
   privileged NewClient/authenticators remain unchanged.
3. On R death, G must stop I and observe its exact kernel exit before releasing
   held input. EOF alone is insufficient if I can still finish an old press.
   Only a native wait timeout may be polled again; a failed handle value must
   not be retried against a potentially reused resource. Missing proof stays
   recovery-unknown and cannot authorize another input owner.
4. On normal/error shutdown, one parent-owned coordinator retires I first,
   then disarms/joins G. Expose one full retirement acknowledgement covering
   both Process lifetimes and all native cleanup/protocol tasks. Reuse the
   existing ExitPending fence; errors.As of two unrelated pending errors must
   not accidentally retain only one child's completion.
5. Add real hidden subprocess trials: kill R while I ignores cancellation after
   an acknowledged down; verify G waits for exact I retirement and releases the
   journal. Also cover denied/unconfirmed termination, partial/unknown receipts,
   ordinary desktop changes, and repeated Close without worker/handle growth.
   No native input is needed for the process-lifetime marker tests.
6. Only then add the private agent child/guardian modes, wire productRunner's
   worker==nil input adapter to the complete process-backed port, and add actual
   desktop/cursor smoke plus paired acceptance. Parent-side geometry/admission
   queries remain separate: never call a SendInput-only split a complete cursor
   ownership fix. Keep normal portable behavior and protected service routing.

No new implementation task was started after this round's ten-minute cutoff.
The full portable split remains unfinished and must be continued, not replaced
by transport-only completion. All ten issues remain open. No push, issue comment,
extra branch/worktree, public API, DB, CI runner, dependency, firewall or signing
change. Only internal source/tests and this progress/evidence record change.
