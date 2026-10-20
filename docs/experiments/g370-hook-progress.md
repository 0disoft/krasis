# g370: bounded hook progress observation

The source owner now independently checks hook progress every 100 ms. An active
callback with no completion for one second gets `hook-callback-stalled`.
Sustained recent physical relative mouse input without physical mouse-hook
progress for two seconds gets `hook-mouse-progress`. Idle periods, completed
short bursts, absolute input and tagged/OS-injected hook traffic cannot turn
into physical hook liveness evidence. These are observed pipeline mismatches,
not proof of hook removal or interference by a particular product.

The monitor uses monotonic durations and atomic updates. It adds no native
calls, I/O, waits or allocations to callbacks. It atomically fences capture
activation and suppression without waiting for a callback-owned state lock.
Cancellation cannot erase its queued failure or skip sender/held-input cleanup.
Existing native authorization is preserved.

A cleanly joined set of native owners carries an internal, unforgeable
retirement marker. The service accepts that marker only without any additional
cleanup failure. Unknown unhook, sender and cleanup outcomes remain fenced;
an old native owner is never replaced while its retirement is unconfirmed.
Fixed cause codes survive the child/service receipt and sanitized support log.

Tests cover idle, short bursts, sustained mismatch, healthy progress, blocked
observers, injected traffic, allocation-free observation, lock-independent
suppression retirement, cancellation races and failed-cleanup proof rejection.
Full wininput/inputworker/agent/productlog/logindiag/residentagent/localapi
tests pass. Existing injection rejection/partial-outcome handling remains
separate from source observation. This completes #86's bounded attribution and
unknown-external-interference handling. #59 still includes removing existing
callback-side native authorization waits; those are not removed by a watchdog.

Reference: https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelmouseproc

No product API, DB, dependency or CI runner change; no Git push. Package and
installed acceptance are recorded after verification.
