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

## Installed acceptance and remaining recovery failure

Both PCs now run generation 370, revision
`858cc31d7bd514543459ccdd00986957dc973a48`. B transfer
`535a2c00ec017e991992accb70d73b4f` reports UPDATED for the verified
27,883,645-byte bundle. A's attended installer completed with exit code 0;
the installed agent SHA-256 matches the bundle. A's CLI, UI, running agent,
installed agent and launch record all match this revision.

The first paired collection at 18:21:46Z/44Z on 2026-10-20 reported input
health and input service READY on both PCs. This did not establish stability:
at 18:22:25Z A recorded `installed_source_reader_eof`, heartbeat response
failure, `fault_source_hook_failed` and `session_failed`. No specific new
hook-progress cause was reported in that failure. These observations do not
identify external interference or prove silent hook removal.

Automatic application recovery restarted A at 18:22:39Z and reconnected at
18:22:41Z. Final collection at 18:23:23Z/22Z again reports input health and
service READY on both PCs; A doctor is healthy/CONNECTED and B is ONLINE.
The short recovered window is not long-term stability or hardware acceptance.

Issue #86 was closed without a comment for the implemented bounded mismatch
classification and safe fencing. Recurring recovery remains unresolved.
Next execution should investigate the service/worker EOF path and its missing
terminal cause, then remove callback-side authorization waits under #59.
Native injection cancellation (#50), network observation (#67/#68/#71/#72),
endpoint hardware acceptance (#70), GUI/release acceptance (#15/#16), and the
physical device matrix (#4/#5/#6/#7 with umbrella #20) remain open. No new
implementation work was started beyond this execution's ten-minute cutoff.

Only main remains locally and remotely, with one worktree and no open PRs.
No new issue comments or Git push were made. Existing package/UI test results
were reused because deployment changed no source. Full hardware fault matrix,
signed release and hosted CI remain unverified. No API, DB, dependency or
runner changes were made by this installation record.
