# g530 source heartbeat and wininput race follow-up

## Observed g529 failures

Fresh paired diagnostics found three A `installed_source_command_failed_heartbeat`
events, one more than the preceding installation receipt:

| Worker exit report UTC | Final operation |
| --- | --- |
| 09:46:22.602 | source-service-query-cancelled |
| 09:46:27.911 | hook-callback-stalled |
| 09:53:22.773 | source-agent-query-capacity |

All three retained the authenticated peer connection. Service/input were READY,
capture/sender/interrupt owners joined, and retirement was `clean_sent`.
They represent failed source commands and capture replacement, not confirmed
service error 1066 or peer reconnects. A cancelled query can be secondary to
capture shutdown; the operation alone does not identify the initial cause.

Existing heartbeat timings aggregate maxima over 15 seconds. Worker buckets
copied from a previous successful reply cannot establish that a later failed
heartbeat spent less than 1ms in its worker. g530 emits a separate, fixed-label
snapshot of each failed attempt; missing/untrusted measurements are explicitly
`unavailable`. Existing aggregate metrics and packet formats remain intact.

## Bounded callback change

An inactive physical WM_MOUSEMOVE previously waited for the same full
service/agent authority proof as actual capture, even though its LL hook only
observed coordinates and passed the movement through. These queries could
occupy the callback and compete with actual capture and heartbeat queries.

g530 makes only this passive hook observation independent of authority waits.
The decision uses a nonblocking physical-gate acquisition and rechecks inactive
state under that gate. Active capture, contended activation, buttons, keyboard,
wheels and a visible return cursor retain the existing checks. Raw Input
forwarding remains independently authorized. Pending stationary cursor snapshots
and prior native cleanup failures retain their existing outcomes. No native call
or hook-chain continuation executes under the new physical gate.

The regression test handles 1,000 passive physical moves with 0 authority guard
calls, versus the previous 1,000 calls. Active capture still checks and suppresses
the next movement; Raw Input emits it once. This removes a concrete blocking
path but does not prove that all three recorded g529 failures had that cause.
The existing LL/Raw activation boundary can observe events on different owner
threads; this change does not claim a new cross-thread event-generation protocol.

## Test corrections

The full wininput race failure was checkptr rejecting test-owned Go memory
converted to uintptr across ordinary Go callback calls. Payloads now use Windows
LocalAlloc memory with registered cleanup, and a test traverses the actual native
callback trampolines. Product pointer checks are not disabled or bypassed.

A native writer admission test also assumed Sleep(20ms) guarantees at least
20,000 QPC microseconds. One full run observed 19,627us. Its gate now waits until
the same QPC clock reaches the target; the original 20,000us assertion remains.

## Verification and limits

Whole-repository Go tests (56 passing packages, 8 without tests), vet (existing
wininput unsafeptr policy), desktop tests, changed callback tests and full
wininput race passed. Targeted final diagnostic/filter checks and installation
evidence are recorded in the paired installation receipt. No public API, DB,
dependency, private packet format, CI runner or architecture source-of-truth
document changed. Generation 530 identifies the locally installed bundle.

Paired installation and CLI connection are authorized. Remote push, remote CI
and public release are not part of this follow-up. Installation health and a
short connected observation are not a replacement for attended crossings or
long-duration testing under CPU load. Recovery or reconnect freedom must not
be inferred beyond the recorded observation interval.

Final paired diagnostics confirmed the same g530 authenticated connection and
READY service/input on both notebooks for approximately 287 seconds, with no
new heartbeat failure, reconnect or recovery event after connection. B's seven
network acquire timeouts before that connection, during update/disconnect and
installation, are retained in the receipt. No attended g530 crossing sample was
collected in this observation.
