# ADR 0065: Bounded handling of a missing foreground window

## Evidence and scope

In the g105 export of 2026-10-01, A recorded four
`activation_capture_foreground_unavailable` events at 17:23:25–28 KST after both
devices connected. Neither export recorded a reconnect in this interval. This
identifies the refused source activation; it does not establish which app or
Windows transition caused the missing window or how long that condition lasted.

Windows documents that `GetForegroundWindow` may return NULL while a window
loses activation. The existing capture fallback immediately rejects that value.
A deterministic fake-native reproduction returns NULL once and then an ordinary
responsive foreground window; g105 rejects before the second observation.

## Decision

Only when the ordinary foreground activation needs its fallback and the current
foreground is NULL, resample at 10 ms intervals up to five times. An already
observed foreground has no added delay. The existing outer capture command
deadline remains in force. Cancellation is checked before and after every pause.

Use only the newly observed handle. Do not reuse saved focus, force activation
without a window, attach to an unknown thread, relax responsiveness probing, or
change protected-desktop rules. A persistent NULL still returns the original
foreground-unavailable classification and uses the existing verified rollback.
Normal activation, thread detach, cursor restoration and input release rules
remain in their original owners.

This improves a transient case; it is not evidence that the reported four
failures were all transient or that physical A-to-B handoff is fixed. The g106
device check must confirm whether the refusal recurs after deployment. No window
titles, input contents, process names or user identifiers are recorded.

## Validation

Fake-native checks cover recovery from one NULL, persistent NULL budget,
cancellation during the pause, no latency on the ordinary path, and continued
refusal of an unresponsive window. Existing foreground/rollback tests must also
pass. Product generation advances to g106; its update layout may carry the
previously implemented installer, but receiving it does not run or install it.

The focused checkptr tests pass. Full `go vet` reports six existing unsafeptr
warnings in unchanged native pointer conversion files; the other vet analyzers
are checked separately with `-unsafeptr=false`. This is not a clean full-vet
claim or a change to those native conversion paths.

## Reference

- [GetForegroundWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getforegroundwindow)

## g132: Request activation when passive polling stays NULL

The 2026-10-03 g131 device exports contain 12 source handoff profiles: two
successful and ten refused with `activation_capture_foreground_unavailable`.
The failed capture preparations took about 55–57 ms, consistent with exhausting
the five observation intervals. The operator reports that a local mouse click
then permits handoff. The exports identify the refused capture stage, not the
Windows-internal reason for losing foreground ownership.

Passive polling is insufficient when there is no foreground owner to restore
itself. Within the same five intervals, resample first; if the result is still
NULL, request `SetForegroundWindow` for the existing capture window and read the
foreground handle again. Windows documents absence of a foreground process as
one condition permitting this request; it also explicitly permits denial.
Success still requires observing the actual capture handle. An API success
without ownership remains a refusal, and a newly observed external window uses
the existing response probe and attach/detach fallback. Cancellation is checked
around each request. No remembered application handle, synthetic click/key,
foreground permission grant, or system-setting change is used.

Regression checks cover recovery without a physical click, API denial,
unverified API success, cancellation inside the request, passive recovery to a
responsive external owner, and the original bounded refusal. Two-device g132
handoff without an extra click remains a manual validation requirement.

- [SetForegroundWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setforegroundwindow)

## g133: Separate visibility from activation

The 03:07 KST g132 exports confirm the exact build on both devices. The bounded
12 profiles contain six successful source handoffs followed by six refused
captures; the event history contains 20 foreground-unavailable activation
events. These profiles are capped and are not a total attempt count or a success
rate. The NULL retries did not resolve the reported device failure.

Previously `ShowWindow(SW_SHOW)` also requested activation before the explicit
foreground request. Use `SW_SHOWNA` to show the capture at its configured size
and position without independently activating it. The subsequent verified
foreground request, responsiveness probe, thread detach and rollback stay
mandatory. Preserving the external owner for fallback is the intended behavior;
g132's exports do not prove that showing the window caused the NULL condition.

On a foreground-unavailable refusal, attach only a fixed classification of the
first observed NULL: `foreground_missing_before_show`,
`foreground_missing_after_show`, or `foreground_missing_after_request`. The
after-show observation occurs after the existing `UpdateWindow` call. These
are observation stages, not proof of Win32 causality. They enter the existing
sanitized activation event, without handles, window titles or process identity.
Other failures keep their existing classification, especially rollback failure.

Fake-native tests cover preserved foreground through showing and verified
attach/detach fallback, each NULL stage, clean refusal and sanitized export.
Click-free A-to-B device handoff remains unverified until g133 is deployed.

- [ShowWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-showwindow)
