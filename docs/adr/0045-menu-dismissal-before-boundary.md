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
Custom application menus that ignore WM_CANCELMODE remain unsupported by this
native-menu path. No global Esc fallback is added. Windows UIPI can block the
message to elevated applications; the existing fail-closed behavior is retained.

Mocked tests cover successful dismissal, rejection, held input, foreground/menu
replacement and remaining capture/move-size states. Native system/context menus,
scrolling and both directions require two-PC testing. Agent generation is 17.

Reference: [WM_CANCELMODE](https://learn.microsoft.com/en-us/windows/win32/winmsg/wm-cancelmode).
