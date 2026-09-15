# ADR 0039: Near-transparent source capture window

- Status: Accepted
- Scope: Windows outbound input capture

## Context

The user observed a white square at A's right edge while A's mouse or touchpad
controls B. B's own touchpad controlling A did not show the same visible square.
The source window from ADR 0023 is a 24-by-24 opaque system-background popup.
This is a code-supported explanation, not yet a hardware-verified diagnosis of
the difference between the two machines.

## Decision

Keep the foreground, touchpad-capable window, dimensions, hit testing, gesture
conversion, wheel ownership, and cleanup unchanged. Create it as WS_EX_LAYERED
and set LWA_ALPHA to 1/255 before it can be shown. Configuration failure aborts
setup and destroys the window instead of falling back to an opaque surface.

Do not use alpha zero, a color key, or WS_EX_TRANSPARENT: those can route pointer
hit testing through the surface. The window is near-transparent, not literally
invisible; a compositor-dependent one-level shade difference may remain.

This does not replace system cursors or hide the destination cursor left after
an inbound session returns. That is a separate lifecycle problem and remains
unresolved by this change.

## Validation

- Unit coverage for nonzero alpha, no color key, and setup failure.
- Existing capture activation, focus restoration, wheel routing, and timeout
  tests remain applicable.
- Manual two-PC check required: both input directions, mouse and touchpad,
  dark/light backgrounds, vertical/horizontal scrolling, repeated return,
  and disconnect. Unit tests do not prove Precision Touchpad delivery through
  a layered foreground window on these devices.

## References

- [Layered windows and hit testing](https://learn.microsoft.com/en-us/windows/win32/winmsg/window-features#layered-windows)
- [SetLayeredWindowAttributes](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setlayeredwindowattributes)
