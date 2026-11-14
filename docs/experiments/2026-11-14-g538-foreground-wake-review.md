# g538: service foreground transitions while waiting

## Evidence and diagnosis

The g537 paired review retained 14 A activations, median 18.888ms and maximum
236.627ms. The maximum contained 230.481ms of worker activation and a nested
95.860ms foreground request/exchange. Its request delivery was only 0.066ms.
Thus network transfer does not explain that particular activation outlier.
The foreground interval includes monitor scheduling, fresh desktop validation,
SetForegroundWindow and owner message service; it is not an isolated API cost.
Position/show and runtime scheduling also contribute, and remain unresolved.

A real attended native smoke on the unmodified path passed five compare cycles.
The monitor-exchange activations ranged from 17.964ms to 67.065ms. One successful
request returned before actual ownership and required the pending-transition
fallback. This is mechanism evidence, not a controlled paired latency comparison.

## Change

When foreground is NULL, the owner previously called time.Sleep(10ms) up to
five times, without servicing activation during those sleeps. It now waits for
new Windows input/system events and immediately pumps the same nonqueued/internal
activation events used by the main request exchange. An observed foreground ends
the wait immediately. Unrelated input neither renews the absolute 10ms retry
budget nor consumes another activation attempt. Five attempts and the outer
two-second command timeout remain unchanged.

The request exchange uses the same message wait with its existing native reply
event and 10ms cancellation cap. QS_ALLINPUT wakes for new input; omitting
MWMO_INPUTAVAILABLE prevents repeated wakeups for previously seen but deliberately
unremoved queued messages. Neither path removes or dispatches queued input,
posted commands or WM_QUIT. Native wait failure preserves the Windows error.

Fresh desktop/authority checks, actual foreground ownership, cancellation fences,
rollback and ACK-before-input remain mandatory. No API, wire format, DB,
dependency or CI runner changes are involved. Generation is 538.

## Validation and limits

Focused tests cover immediate activation wakeup, repeated unrelated input with a
shrinking deadline, exact deadline expiry, cancellation before a pump or native
read, pump-time cancellation, Windows wait failure, reply-event wiring, queue
flags and upward millisecond rounding. Existing bounded refusal tests remain.

The deterministic activation case completes on the first 2ms simulated wake,
instead of retaining the old fixed 10ms sleep. This removes up to the remaining
sleep interval per successful wake; it does not prove that the complete paired
236.627ms or 95.860ms tails are eliminated. Physical paired latency requires a
fresh user crossing trial after installation. Native smoke observations and
installation health are separate from that trial.

Focused foreground tests, uncached inputworker/wininput/residentagent tests
(excluding PortablePair), scoped inputworker/wininput race/checkptr, vet with
the repository's existing -unsafeptr=false setting, and diff hygiene passed.
The changed attended native smoke passed all five activation/ownership/retirement
cycles: activation 31.189..45.524ms and foreground exchange 3.014..8.288ms.
All five required legitimate API-rejection fallback; this smoke does not isolate
the NULL-transition improvement and is not a measured paired latency win.
Full repository/Linux tests and remote CI are omitted because this change is
Windows-only capture waiting, with no transport or Linux implementation change.

Rollback uses the normal verified installer/update workflow with a new generation
containing the previous source; never overwrite an existing generation artifact.

Windows semantics: [MsgWaitForMultipleObjectsEx](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-msgwaitformultipleobjectsex)
and [PeekMessageW](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-peekmessagew).
