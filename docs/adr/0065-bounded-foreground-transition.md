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
