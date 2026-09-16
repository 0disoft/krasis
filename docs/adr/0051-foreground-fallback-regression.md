# ADR 0051: Restore guarded foreground compatibility

Generation 18 disabled the existing foreground thread-attachment fallback. On the
reported machine connection succeeded, the first boundary attempt was recorded,
then the session faulted. The sanitized log omitted its terminal failure reason,
so that report alone does not prove the exact native error. Static inspection
does prove that the previously working fallback became an unconditional failure.

Generation 19 restores that compatibility path, with a 100 ms WM_NULL response
probe before attaching to the foreground thread. The probe rejects an already
unresponsive window; it cannot prevent a later hang. Attachment is detached after
the foreground attempt, including when terminal cancellation was noticed after
attachment. Command deadlines, permanent retirement, owner-thread cleanup, and
the conservative FAULTED latch remain in place.

This supersedes only the blanket thread-attachment disablement in ADR 0050.
An unresponsive foreign UI can still stall a native call after the probe. That
case requires process-level recovery; this patch is not a bounded-native-call
guarantee. Microsoft documents this shared-input-queue risk:
https://devblogs.microsoft.com/oldnewthing/20080801-00/?p=21393

Terminal unverified failures now preserve the existing typed descriptor before
the supervisor returns. The resident log exports closed-vocabulary `fault_*`
codes even during DISCONNECTING. No arbitrary error strings are exported.

Regression tests cover native attachment flags, probe denial, retained terminal
descriptors, stale observer rejection, and sanitized export. Two-PC first crossing,
return, cursor hiding, and touchpad scrolling still require hardware verification.
