# ADR 0045: Dismiss a native menu before crossing the boundary

Status: Accepted

Instead of unconditionally rejecting native menu mode, boundary admission sends
WM_CANCELMODE to the reported nonzero menu owner with a 150ms SendMessageTimeout
limit. No key injection, broadcast, EndMenu on the wrong thread or window close
is used. Pressed input and move/size interaction prevent this dismissal attempt.

Admission then verifies menu exit for at most 150ms, with context cancellation.
Changed foreground/menu owner, timeout or message failure still block crossing.
After confirmed exit, capture, move/size and pressed-input checks run again.
Existing source activation verification and rollback remain unchanged.

A timed-out window procedure can complete later; a timeout never grants control.
In g67, an idle foreground GUI thread with mouse capture but no native menu
flag also receives a bounded WM_CANCELMODE at its capture window. Boundary
admission proceeds only after the same active window reports no capture, menu,
or move/size state and no physical input is held. Failed delivery, changed
foreground, retained or transferred capture, and new menu state still block
crossing. This targets capture-backed custom menus without cancelling an active
drag. Custom menus that expose neither native menu state nor mouse capture, or
ignore WM_CANCELMODE, remain unsupported by this path. No global Esc fallback
is added. Windows UIPI can block the message to elevated applications; the
existing fail-closed behavior is retained.

Mocked tests cover successful dismissal, rejection, held input, foreground/menu
replacement and remaining capture/move-size states. Native system/context menus,
scrolling and both directions require two-PC testing. Agent generation is 17.

Reference: [WM_CANCELMODE](https://learn.microsoft.com/en-us/windows/win32/winmsg/wm-cancelmode).
