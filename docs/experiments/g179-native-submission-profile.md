# g179 native submission profile

## Question and measurement boundaries

The g178 B worst retained input had dispatch 22.451ms and worker apply 18.602ms,
with no cursor snapshot on that operation. g179 measures the native `SendInput`
call inside that same worker apply, before changing input scheduling or authority.

The existing `workerApply` receipt retains disjoint `micros[apply,snapshot]` and
their presence/saturation masks. It now optionally includes:

- `submissionMicros`: wall duration around `callSendInput` using the worker's
  monotonic clock, floored to microseconds and capped at the 120-second window.
- `submissionCalls`: 0 for absent, 1 for one measured call. Measured zero has
  calls 1. This counts native calls, never injected events, keys or buttons.
- `submissionSaturated`: true only when the native duration exceeded the cap.

Native duration is nested inside apply and must not be added to it or to client
response. It must fit inside the containing measured apply. An operation that
does not call `SendInput` has no submission receipt. Early validation/cancellation
does not fabricate a native call. Failed and partially applied calls retain their
original outcome and call timing. If future input code makes multiple submissions
within one apply, diagnostics are invalidated rather than silently retaining one
call; the input ACK and cursor suffix still keep their original meaning.

Only the already bounded receiver/installed input samples request this addition:
at most 2,048 probes across the existing twelve-attempt, 120-second collector.
The worker attaches the probe to the synchronous apply context only; it is absent
from subsequent cursor snapshot, bootstrap, heartbeat and recovery. No input
payload, operation kind, coordinate, process identity or window content is added.

## Local wire compatibility

KRI1 v2 retains its 48-byte request. Byte 7 is 1 for legacy apply/snapshot detail
or 3 for that detail plus native submission. Other flag values and maintenance
requests are rejected before applying input. Legacy requests still receive their
legacy detail shape; they cannot inadvertently enable native submission timing.

KRA1 v5 adds an eight-byte native block after the existing twelve-byte v4 detail:
four-byte big-endian microseconds, one-byte call presence/count, one-byte saturation
(0 or 1), and two reserved zero bytes. Private cursor coordinates follow the
extension as before. The response remains one write, one ACK and one round trip;
cursor responses retain one header read and one combined suffix read. New readers
continue to accept v1-v4. A new native-detail request requires a matching upgraded
installed worker; old readers/workers are not forward compatible with the flag or
v5. Existing installed-image verification and protected update are retained.

Truncated data, unknown saturation flags and nonzero reserved bytes are structural
protocol failures. Invalid numeric/presence/parent bounds are consumed and dropped
without changing acknowledged input certainty or shifting the next ACK. No input
replay, alternate backend fallback or authority caching is introduced.

## Verification and device follow-up

Hosted tests cover native success, rejection, partial/unknown outcome, cancellation
before submission, disabled timing, operation context isolation, nested bounds,
saturation, explicit opt-in, v5 cursor/legacy alignment, numeric metadata drop,
structural rejection and sanitized local/peer exports. Native calls in these
tests use a stub; they do not interact with a real PIN dialog.

After both devices and their protected workers are on g179, use ordinary movement
and boundary crossings, then export both diagnostics. Compare `submissionMicros`
with its containing apply on the same first/worst vector, retaining missing and
saturated observations. Do not infer a whole-session distribution from retained
first/worst samples or add timings from different operations/hosts. Native PIN
reentry and actual call latency require device evidence. This build adds diagnostic
resolution; it does not claim a performance improvement.
