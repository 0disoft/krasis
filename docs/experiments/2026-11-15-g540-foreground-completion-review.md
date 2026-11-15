# g540: avoid redundant work after foreground completion

## Evidence and bounded change

g539 collection `40d88707c621af17cf1fb3442830d788` has an A capture maximum
67.755ms. Its matching worker activation is 61.941ms, including foreground
request/exchange 47.028ms. Request delivery is 0.131ms and reply delivery is
0.089ms. These nested intervals do not identify the whole tail as a pipe stall,
one Windows API, or scheduler delay. No new reconnect/recovery was retained.

The foreground exchange always called PeekMessage after a successful requester
reply, even when its own HWND already owned foreground. If the requester
completed while the owner was servicing internal activation, it also entered
MsgWaitForMultipleObjectsEx before checking the reply again. PM_NOREMOVE avoids
removing posted messages but can still execute sent callbacks; an extra pump
is not a harmless queue inspection.

g540 checks for that completion before the next native wait. Completion retains
native errors, cancellation, absolute expiry and terminal checks. It then samples
GetForegroundWindow and skips the final pump only for its own nonzero HWND.
Pending/foreign ownership still pumps. The activation caller independently
checks ownership immediately before active commit; an intervening owner change
retains fallback and rollback. No saved foreground sample authorizes capture.
The requester must still complete its native call, and the original two-reference
event ownership closes the handle only after both owners finish.

The deterministic completion-during-pump fixture removes one native wait and
reduces two pumps to one; a completed owned-window reply needs zero final pumps
instead of one. No timer budget, authentication, lease, wire frame, ACK layout,
input suppression or authority policy changes. This is one capture-exchange
target, plus tests, generation and its runtime documentation.

## Verification

Uncached wininput/inputworker/handoff/residentagent/agent tests passed.
PortablePair fixtures were excluded while the real paired service is running.
Scoped wininput race/checkptr tests passed, including cancellation, pending
activation, native error propagation, late caller/event retirement, bounded
commands and the new no-wait/no-duplicate-pump cases. A dedicated changed-owner
fixture verifies that shortcut completion cannot authorize active capture after
another application takes foreground. Vet passed with repository unsafeptr=false
policy. The first race invocation had CGO disabled; the successful run explicitly
enabled CGO and used the existing portable LLVM-MinGW compiler.

Real Windows foreground smoke was exercised. One initial fixed run failed on
its third activation with SetForegroundWindow rejection and AttachThreadInput
access denied. This refusal remains fail-closed; it is not hidden as a pass or
proof of a native timing improvement. Subsequent original-source overlay and
fixed-source five-activation runs both passed ownership/deactivation checks.
Original median/max activation was 49.956/80.207ms; fixed was 37.738/80.938ms.
Foreground apps, scheduling and fallback paths differed, so these five-sample
runs do not establish a causal improvement or a lower maximum. Initial native
tracing also observed a sub-ms API return inside a tens-of-ms exchange; its
temporary instrumentation was removed. No native trace contains raw input.

The 67ms production tail is not claimed solved by these local tests. A new
paired physical trial on the exact installed build must compare A activation,
foreground exchange, B dispatch and fault/reconnect records. Long-duration
capture retirement remains a separate validation limit.

## Delivery policy

Use one clean g540 revision for workspace bundle, protected-input installer and
verified setup. Preserve the exact peer update receipt, install both notebooks,
verify generation/revision/READY, then restore CLI connection. Existing artifact
generations are never overwritten. Rollback is a new generation built from the
previous source through the same verified installer workflow. Push, public
release and remote CI are outside this request. Source of truth updated:
docs/architecture/02-runtime-flow.md. No API, DB, dependency, hygiene or runner
changes.
